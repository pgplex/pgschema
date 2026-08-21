package postgres

import (
	"strings"
	"unicode"
)

// QualifiedName is a schema-qualified table name extracted from SQL.
type QualifiedName struct {
	Schema string
	Table  string
}

// keywords that cannot be an unquoted table name immediately after REFERENCES
// (e.g. GRANT REFERENCES ON TABLE ...).
var referencesFollowKeywords = map[string]bool{
	"on":     true,
	"to":     true,
	"from":   true,
	"where":  true,
	"set":    true,
	"all":    true,
	"table":  true,
	"schema": true,
}

// ExtractForeignKeyTargets returns schema-qualified table names that appear
// as REFERENCES targets in sql. Unqualified names use defaultSchema.
// String literals, comments, and dollar-quoted bodies are skipped.
func ExtractForeignKeyTargets(sql, defaultSchema string) []QualifiedName {
	seen := make(map[string]bool)
	var out []QualifiedName

	walkSQLCode(sql, func(code string) {
		i := 0
		for i < len(code) {
			idx := indexKeyword(code, i, "references")
			if idx < 0 {
				return
			}
			i = idx + len("references")
			schema, table, next, ok := parseQualifiedName(code, i)
			if !ok {
				i = next
				continue
			}
			i = next
			if referencesFollowKeywords[table] {
				continue
			}
			if schema == "" {
				schema = defaultSchema
			}
			key := schema + "." + table
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, QualifiedName{Schema: schema, Table: table})
		}
	})

	return out
}

// ExtractCreateTableNames returns schema-qualified table names from CREATE TABLE
// statements in sql. Unqualified names use defaultSchema.
func ExtractCreateTableNames(sql, defaultSchema string) []QualifiedName {
	seen := make(map[string]bool)
	var out []QualifiedName

	walkSQLCode(sql, func(code string) {
		i := 0
		for i < len(code) {
			idx := indexKeyword(code, i, "create")
			if idx < 0 {
				return
			}
			i = idx + len("create")
			i = skipSpace(code, i)
			// Optional TEMP/TEMPORARY/UNLOGGED/GLOBAL/LOCAL modifiers before TABLE
			for {
				word, next, ok := parseUnquotedIdent(code, i)
				if !ok {
					break
				}
				switch word {
				case "global", "local", "temp", "temporary", "unlogged":
					i = next
					i = skipSpace(code, i)
				default:
					goto afterMods
				}
			}
		afterMods:
			if !hasKeywordAt(code, i, "table") {
				continue
			}
			i += len("table")
			i = skipSpace(code, i)
			if hasKeywordAt(code, i, "if") {
				i += 2
				i = skipSpace(code, i)
				if hasKeywordAt(code, i, "not") {
					i += 3
					i = skipSpace(code, i)
					if hasKeywordAt(code, i, "exists") {
						i += 6
						i = skipSpace(code, i)
					}
				}
			}
			if hasKeywordAt(code, i, "only") {
				i += 4
				i = skipSpace(code, i)
			}
			schema, table, next, ok := parseQualifiedName(code, i)
			i = next
			if !ok || table == "" {
				continue
			}
			if schema == "" {
				schema = defaultSchema
			}
			key := schema + "." + table
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, QualifiedName{Schema: schema, Table: table})
		}
	})

	return out
}

// ExtractPartitionOfTargets returns schema-qualified table names that appear
// as PARTITION OF targets in sql. Unqualified names use defaultSchema.
// String literals, comments, and dollar-quoted bodies are skipped.
func ExtractPartitionOfTargets(sql, defaultSchema string) []QualifiedName {
	seen := make(map[string]bool)
	var out []QualifiedName

	walkSQLCode(sql, func(code string) {
		i := 0
		for i < len(code) {
			idx := indexKeyword(code, i, "partition")
			if idx < 0 {
				return
			}
			i = idx + len("partition")
			i = skipSpace(code, i)
			if !hasKeywordAt(code, i, "of") {
				continue
			}
			i += len("of")
			schema, table, next, ok := parseQualifiedName(code, i)
			if !ok {
				i = next
				continue
			}
			i = next
			if schema == "" {
				schema = defaultSchema
			}
			key := schema + "." + table
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, QualifiedName{Schema: schema, Table: table})
		}
	})

	return out
}

func walkSQLCode(sql string, fn func(code string)) {
	for _, seg := range splitDollarQuotedSegments(sql) {
		if seg.quoted {
			continue
		}
		walkSQLCodePreservingStringsAndComments(seg.text, fn)
	}
}

func walkSQLCodePreservingStringsAndComments(text string, fn func(code string)) {
	i := 0
	segStart := 0
	flushCode := func(end int) {
		if end > segStart {
			fn(text[segStart:end])
		}
		segStart = end
	}

	for i < len(text) {
		ch := text[i]

		if ch == '\'' {
			flushCode(i)
			i++
			for i < len(text) {
				if text[i] == '\'' {
					if i+1 < len(text) && text[i+1] == '\'' {
						i += 2
					} else {
						i++
						break
					}
				} else {
					i++
				}
			}
			segStart = i
			continue
		}

		if ch == '-' && i+1 < len(text) && text[i+1] == '-' {
			flushCode(i)
			i += 2
			for i < len(text) && text[i] != '\n' {
				i++
			}
			if i < len(text) {
				i++
			}
			segStart = i
			continue
		}

		if ch == '/' && i+1 < len(text) && text[i+1] == '*' {
			flushCode(i)
			i += 2
			for i < len(text) {
				if text[i] == '*' && i+1 < len(text) && text[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			segStart = i
			continue
		}

		i++
	}
	flushCode(i)
}

// indexKeyword finds the next occurrence of keyword as a whole word, case-insensitive.
func indexKeyword(s string, start int, keyword string) int {
	n := len(keyword)
	for i := start; i+n <= len(s); i++ {
		if hasKeywordAt(s, i, keyword) {
			return i
		}
	}
	return -1
}

func hasKeywordAt(s string, i int, keyword string) bool {
	n := len(keyword)
	if i+n > len(s) {
		return false
	}
	if !strings.EqualFold(s[i:i+n], keyword) {
		return false
	}
	if i > 0 && isIdentChar(rune(s[i-1])) {
		return false
	}
	if i+n < len(s) && isIdentChar(rune(s[i+n])) {
		return false
	}
	return true
}

func skipSpace(s string, i int) int {
	for i < len(s) && unicode.IsSpace(rune(s[i])) {
		i++
	}
	return i
}

func parseQualifiedName(s string, i int) (schema, table string, next int, ok bool) {
	i = skipSpace(s, i)
	first, i, ok := parseIdent(s, i)
	if !ok {
		return "", "", i, false
	}
	i = skipSpace(s, i)
	if i < len(s) && s[i] == '.' {
		i++
		second, j, ok2 := parseIdent(s, i)
		if !ok2 {
			return "", "", i, false
		}
		i = j
		i = skipSpace(s, i)
		if i < len(s) && s[i] == '.' {
			// catalog.schema.table
			i++
			third, k, ok3 := parseIdent(s, i)
			if !ok3 {
				return "", "", i, false
			}
			return second, third, k, true
		}
		return first, second, i, true
	}
	return "", first, i, true
}

func parseIdent(s string, i int) (string, int, bool) {
	i = skipSpace(s, i)
	if i >= len(s) {
		return "", i, false
	}
	if s[i] == '"' {
		return parseQuotedIdent(s, i)
	}
	return parseUnquotedIdent(s, i)
}

func parseQuotedIdent(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '"' {
		return "", i, false
	}
	i++
	var b strings.Builder
	for i < len(s) {
		if s[i] == '"' {
			if i+1 < len(s) && s[i+1] == '"' {
				b.WriteByte('"')
				i += 2
				continue
			}
			return b.String(), i + 1, b.Len() > 0
		}
		b.WriteByte(s[i])
		i++
	}
	return "", i, false
}

func parseUnquotedIdent(s string, i int) (string, int, bool) {
	i = skipSpace(s, i)
	if i >= len(s) {
		return "", i, false
	}
	if !isIdentStart(rune(s[i])) {
		return "", i, false
	}
	start := i
	i++
	for i < len(s) && isIdentChar(rune(s[i])) {
		i++
	}
	return strings.ToLower(s[start:i]), i, true
}

func isIdentStart(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_'
}

func isIdentChar(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9') || r == '$'
}
