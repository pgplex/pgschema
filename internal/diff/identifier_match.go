package diff

import (
	"regexp"
	"strings"
	"sync"
)

// Textual dependency checks (does this view read that table, does this
// expression name that column) run over SQL text produced by PostgreSQL's
// deparsers: pg_get_viewdef, pg_get_expr, pg_get_indexdef, pg_get_triggerdef.
// Those render an identifier either bare (all-lowercase names) or
// double-quoted with embedded quotes doubled ("a""b"). Every such check goes
// through identifierRegexp so the two spellings, identifier boundaries, string
// literals, function calls, and type casts are handled in one place (#591).

// identifierMatchMode selects the boundary rules for a match.
type identifierMatchMode int

const (
	// relationMatch matches a relation (table or view) name; a following "("
	// or a preceding "::" still counts, e.g. "INTO t(...)" or "::t" rowtype casts.
	relationMatch identifierMatchMode = iota
	// columnMatch matches a column reference inside an expression; a name
	// followed by "(" is a function call and one preceded by ":" is a type
	// cast, neither of which is a column.
	columnMatch
)

// sqlStringLiteralRegex matches a single-quoted SQL string literal, including
// doubled-quote escapes.
var sqlStringLiteralRegex = regexp.MustCompile(`'(?:[^']|'')*'`)

// identifierRegexpCache memoizes compiled patterns; view and expression
// dependency checks run the same names over many definitions.
var identifierRegexpCache sync.Map // map[string]*regexp.Regexp

// identifierSpellings renders the two ways a deparser can spell name: bare, or
// double-quoted with embedded quotes doubled.
func identifierSpellings(name string) string {
	bare := regexp.QuoteMeta(name)
	quoted := regexp.QuoteMeta(`"` + strings.ReplaceAll(name, `"`, `""`) + `"`)
	return `(?:` + bare + `|` + quoted + `)`
}

// identifierRegexp returns a pattern matching name as a whole identifier in
// deparsed SQL. parts holds the name's qualification segments (schema, name)
// or a single unqualified name; each segment may appear bare or quoted.
func identifierRegexp(mode identifierMatchMode, parts ...string) *regexp.Regexp {
	key := string(rune('0'+mode)) + "\x00" + strings.Join(parts, "\x00")
	if re, ok := identifierRegexpCache.Load(key); ok {
		return re.(*regexp.Regexp)
	}

	spellings := make([]string, len(parts))
	for i, part := range parts {
		spellings[i] = identifierSpellings(part)
	}
	body := strings.Join(spellings, `\.`)

	// Identifier characters never border a whole-identifier match, and a
	// quote cannot either, so a bare spelling never matches inside a quoted
	// identifier. A qualified name (or a name that itself contains a dot)
	// must not sit inside a longer path such as other.schema.name.
	dotted := len(parts) > 1
	for _, part := range parts {
		dotted = dotted || strings.Contains(part, ".")
	}
	before, after := `[^\w$"]`, `[^\w$"]`
	if dotted {
		before, after = `[^\w$".]`, `[^\w$".]`
	}
	if mode == columnMatch {
		before = before[:len(before)-1] + `:]`
		after = after[:len(after)-1] + `(]`
	}
	re := regexp.MustCompile(`(?i)(?:^|` + before + `)` + body + `(?:` + after + `|$)`)
	identifierRegexpCache.Store(key, re)
	return re
}

// stripStringLiterals blanks out single-quoted literals so their contents
// cannot look like identifiers.
func stripStringLiterals(sqlText string) string {
	if !strings.Contains(sqlText, "'") {
		return sqlText
	}
	return sqlStringLiteralRegex.ReplaceAllString(sqlText, "''")
}

// containsIdentifier reports whether sqlText mentions identifier as a whole
// relation name, in bare or quoted form; "foo" does not match "foobar". An
// identifier containing a dot is tried both as one name (a quoted "a.b") and
// as "schema.name" matched segment by segment, so "my schema"."a""b" is found
// for `my schema.a"b` and "other.foo.bar" does not match "foo.bar". Callers
// that hold schema and name separately should use containsQualifiedIdentifier.
func containsIdentifier(sqlText, identifier string) bool {
	if sqlText == "" || identifier == "" {
		return false
	}
	sqlText = stripStringLiterals(sqlText)
	if identifierRegexp(relationMatch, identifier).MatchString(sqlText) {
		return true
	}
	if schema, name, ok := strings.Cut(identifier, "."); ok {
		return identifierRegexp(relationMatch, schema, name).MatchString(sqlText)
	}
	return false
}

// containsQualifiedIdentifier reports whether sqlText mentions schema.name as
// a whole qualified relation name, each segment bare or quoted.
func containsQualifiedIdentifier(sqlText, schema, name string) bool {
	if sqlText == "" || schema == "" || name == "" {
		return false
	}
	return identifierRegexp(relationMatch, schema, name).MatchString(stripStringLiterals(sqlText))
}

// exprReferencesAnyColumn reports whether a deparsed SQL expression names any
// of the columns as a bare or quoted identifier. Callers pair a positive
// result with IF EXISTS drops and a re-create from the desired state, so a
// false positive costs a redundant drop + create while a miss would leave a
// dependent object behind. (#591)
func exprReferencesAnyColumn(expr string, columns map[string]bool) bool {
	if expr == "" || len(columns) == 0 {
		return false
	}
	expr = stripStringLiterals(expr)
	for column := range columns {
		if identifierRegexp(columnMatch, column).MatchString(expr) {
			return true
		}
	}
	return false
}
