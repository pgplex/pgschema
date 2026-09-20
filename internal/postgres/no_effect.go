package postgres

import (
	"strings"
)

// NoEffectKind names a kind of statement pgschema accepts in a schema file but
// never plans, because the state it sets is outside what inspection reads.
type NoEffectKind string

const (
	// NoEffectOwner is ALTER <object> ... OWNER TO: object ownership follows the
	// pg_dump --no-owner model and is never compared (issue #602).
	NoEffectOwner NoEffectKind = "owner"
	// NoEffectGlobalDefaultPrivileges is ALTER DEFAULT PRIVILEGES without
	// IN SCHEMA: a database-wide default ACL, not schema-level state (issue #603).
	NoEffectGlobalDefaultPrivileges NoEffectKind = "global_default_privileges"
)

// NoEffectStatement is one statement in the desired-state SQL that cannot
// change the plan.
type NoEffectStatement struct {
	Kind NoEffectKind
	SQL  string // statement text with whitespace collapsed, without the trailing ';'
	// Partial is set when only one action of the statement has no effect: an
	// OWNER TO sharing an ALTER TABLE with other actions. The rest still plans.
	Partial bool
}

// ownerAlterKinds are the ALTER targets whose OWNER TO is reported, mapped to the
// keyword that must follow for two-word kinds. ALTER SCHEMA, ALTER DATABASE,
// ALTER FOREIGN DATA WRAPPER and the like are left alone: they are not
// schema-level objects.
var ownerAlterKinds = map[string]string{
	"table": "", "view": "", "index": "", "sequence": "",
	"function": "", "procedure": "", "routine": "", "aggregate": "",
	"type": "", "domain": "",
	"materialized": "view", "foreign": "table",
}

// ownerNameIntroducers precede an identifier, so an `owner` that follows one is
// a column, attribute or constraint being renamed, not the OWNER TO action.
var ownerNameIntroducers = map[string]bool{
	"rename": true, "column": true, "attribute": true, "constraint": true,
}

// StripNoEffectStatements finds the statements in sql that cannot affect the
// plan and returns sql without them, so they neither fail nor leave state behind
// in the plan database. A global ALTER DEFAULT PRIVILEGES would otherwise persist
// on an external plan database, and an OWNER TO role is not stubbed there.
//
// The scan is textual and best-effort: string literals, comments and
// dollar-quoted bodies are skipped, so statements inside a DO block or function
// body are not seen. When OWNER TO shares an ALTER TABLE with other actions, only
// that action is removed.
func StripNoEffectStatements(sql string) (string, []NoEffectStatement) {
	masked := maskNonCode(sql)

	var found []NoEffectStatement
	out := []byte(sql)
	for _, span := range splitStatementSpans(masked) {
		tokens := tokenize(masked[span.start:span.end])
		kind, cut, partial := classifyNoEffect(tokens)
		if kind == "" {
			continue
		}
		first := span.start + tokens[0].pos
		found = append(found, NoEffectStatement{
			Kind:    kind,
			SQL:     strings.Join(strings.Fields(masked[first:span.end]), " "),
			Partial: partial,
		})
		// Blank in place, so leading comments stay and PostgreSQL error
		// positions still point into the user's file.
		from, to := span.start+cut.start, span.start+cut.end
		if !partial {
			to = span.end
			if to < len(sql) && sql[to] == ';' {
				to++
			}
		}
		copy(out[from:to], blankKeepingNewlines(sql[from:to]))
	}
	return string(out), found
}

// classifyNoEffect reports whether the statement is one pgschema never plans,
// and the range of it to drop from the plan database SQL: the whole statement,
// or with partial set, just the OWNER TO action among others.
func classifyNoEffect(tokens []sqlToken) (kind NoEffectKind, cut span, partial bool) {
	if len(tokens) < 3 || !tokens[0].isKeyword("alter") {
		return "", span{}, false
	}
	whole := span{tokens[0].pos, tokens[len(tokens)-1].end}

	if tokens[1].isKeyword("default") && tokens[2].isKeyword("privileges") {
		for i := 3; i+1 < len(tokens); i++ {
			if tokens[i].isKeyword("in") && tokens[i+1].isKeyword("schema") {
				return "", span{}, false
			}
		}
		return NoEffectGlobalDefaultPrivileges, whole, false
	}

	second, ok := ownerAlterKinds[tokens[1].text]
	if !ok || tokens[1].quoted || (second != "" && !tokens[2].isKeyword(second)) {
		return "", span{}, false
	}
	isComma := func(i int) bool {
		return i < len(tokens) && tokens[i].depth == 0 && tokens[i].isKeyword(",")
	}
	owner := -1
	multiAction := false
	for i := 2; i < len(tokens); i++ {
		if isComma(i) {
			multiAction = true
		}
		// OWNER TO <role>: the role is one token (name, quoted name, CURRENT_USER).
		if owner < 0 && tokens[i].depth == 0 && tokens[i].isKeyword("owner") &&
			i+2 < len(tokens) && tokens[i+1].isKeyword("to") &&
			!(ownerNameIntroducers[tokens[i-1].text] && !tokens[i-1].quoted) {
			owner = i
		}
	}
	if owner < 0 {
		return "", span{}, false
	}
	if !multiAction {
		return NoEffectOwner, whole, false
	}
	// Drop the action together with the comma that joined it to its neighbor.
	cut = span{tokens[owner].pos, tokens[owner+2].end}
	if isComma(owner + 3) {
		cut.end = tokens[owner+3].end
	} else if isComma(owner - 1) {
		cut.start = tokens[owner-1].pos
	}
	return NoEffectOwner, cut, true
}

// maskNonCode returns sql with string literals, comments and dollar-quoted
// bodies overwritten by spaces. Length and newlines are unchanged, so offsets
// into the result are offsets into sql.
func maskNonCode(sql string) string {
	out := []byte(blankKeepingNewlines(sql))
	offset := 0
	for _, seg := range splitDollarQuotedSegments(sql) {
		if !seg.quoted {
			base := offset
			walkSQLCodeSpans(seg.text, func(start, end int) {
				copy(out[base+start:base+end], seg.text[start:end])
			})
		}
		offset += len(seg.text)
	}
	return string(out)
}

func blankKeepingNewlines(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c != '\n' && c != '\r' {
			out[i] = ' '
		}
	}
	return string(out)
}

type span struct{ start, end int }

// splitStatementSpans splits masked SQL on ';', ignoring any inside a
// double-quoted identifier. Spans exclude the ';' itself.
func splitStatementSpans(masked string) []span {
	var spans []span
	start := 0
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '"':
			if _, next, ok := parseQuotedIdent(masked, i); ok {
				i = next - 1
			}
		case ';':
			spans = append(spans, span{start, i})
			start = i + 1
		}
	}
	if start < len(masked) {
		spans = append(spans, span{start, len(masked)})
	}
	return spans
}

// sqlToken is an identifier/keyword (lowercased unless quoted) or a single
// punctuation character, with its offset and parenthesis depth.
type sqlToken struct {
	text   string
	quoted bool
	pos    int // offset of the first byte
	end    int // offset past the last byte
	depth  int
}

func (t sqlToken) isKeyword(keyword string) bool {
	return !t.quoted && t.text == keyword
}

func tokenize(s string) []sqlToken {
	var tokens []sqlToken
	depth := 0
	for i := skipSpace(s, 0); i < len(s); i = skipSpace(s, i) {
		if s[i] == '"' {
			if text, next, ok := parseQuotedIdent(s, i); ok {
				tokens = append(tokens, sqlToken{text: text, quoted: true, pos: i, end: next, depth: depth})
				i = next
				continue
			}
		}
		if text, next, ok := parseUnquotedIdent(s, i); ok {
			tokens = append(tokens, sqlToken{text: text, pos: i, end: next, depth: depth})
			i = next
			continue
		}
		if s[i] == ')' && depth > 0 {
			depth--
		}
		tokens = append(tokens, sqlToken{text: s[i : i+1], pos: i, end: i + 1, depth: depth})
		if s[i] == '(' {
			depth++
		}
		i++
	}
	return tokens
}
