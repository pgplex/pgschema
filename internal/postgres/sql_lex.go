package postgres

import (
	"fmt"
	"strings"
)

// schemaSQLToken retains original spelling so remapping can preserve comments,
// whitespace, ordinary literals, and destination identifiers verbatim.
type schemaSQLToken struct {
	text    string
	kind    byte
	unicode bool
	content string
	trivia  string // Whitespace and comments around a consumed UESCAPE clause.
	err     error
}

func sqlIdentifierByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 128
}
func lexSQL(text string) []schemaSQLToken {
	var tokens []schemaSQLToken
	for i := 0; i < len(text); {
		start := i
		kind := byte('p')
		switch {
		case strings.ContainsRune(" \t\r\n\f", rune(text[i])):
			kind = 'w'
			for i < len(text) && strings.ContainsRune(" \t\r\n\f", rune(text[i])) {
				i++
			}
		case strings.HasPrefix(text[i:], "--"):
			kind = 'c'
			for i < len(text) && text[i] != '\n' && text[i] != '\r' {
				i++
			}
		case strings.HasPrefix(text[i:], "/*"):
			kind = 'c'
			i += 2
			depth := 1
			for i < len(text) && depth > 0 {
				if strings.HasPrefix(text[i:], "/*") {
					depth++
					i += 2
				} else if strings.HasPrefix(text[i:], "*/") {
					depth--
					i += 2
				} else {
					i++
				}
			}
		case (text[i] == 'U' || text[i] == 'u') && i+2 < len(text) && text[i+1] == '&' && (text[i+2] == '\'' || text[i+2] == '"'):
			quote := text[i+2]
			kind = 's'
			if quote == '"' {
				kind = 'i'
			}
			i += 3
			closed := false
			for i < len(text) {
				if text[i] == quote {
					i++
					if i < len(text) && text[i] == quote {
						i++
					} else {
						closed = true
						break
					}
				} else {
					i++
				}
			}
			if !closed {
				kind = '!'
			}
		case text[i] == '\'' || (text[i] == 'E' || text[i] == 'e') && i+1 < len(text) && text[i+1] == '\'':
			kind = 's'
			closed := false
			escape := text[i] != '\''
			if escape {
				i++
			}
			i++
			for i < len(text) {
				if escape && text[i] == '\\' {
					i++
					if i < len(text) {
						i++
					}
				} else if text[i] == '\'' {
					i++
					if i < len(text) && text[i] == '\'' {
						i++
					} else {
						closed = true
						break
					}
				} else {
					i++
				}
			}
			if !closed {
				kind = '!'
			}
		case text[i] == '"':
			kind = 'i'
			_, end, ok := parseQuotedIdent(text, i)
			i = end
			if !ok {
				kind = '!'
			}
		case text[i] == '$':
			loc := dollarQuoteRe.FindStringIndex(text[i:])
			if loc != nil && loc[0] == 0 {
				kind = 'd'
				tag := text[i : i+loc[1]]
				i += len(tag)
				if end := strings.Index(text[i:], tag); end >= 0 {
					i += end + len(tag)
				} else {
					i = len(text)
				}
			} else {
				i++
			}
		case sqlIdentifierByte(text[i]):
			kind = 'i'
			i++
			for i < len(text) && (sqlIdentifierByte(text[i]) || text[i] >= '0' && text[i] <= '9' || text[i] == '$') {
				i++
			}
		default:
			i++
		}
		tokens = append(tokens, schemaSQLToken{text: text[start:i], kind: kind})
	}
	return decodeUnicodeTokens(tokens)
}

func tokenIdentifier(t schemaSQLToken) string {
	if t.kind != 'i' {
		return ""
	}
	if t.unicode {
		return t.content
	}
	if strings.HasPrefix(t.text, `"`) {
		name, _, _ := parseQuotedIdent(t.text, 0)
		return name
	}
	return strings.ToLower(t.text)
}

func completeSQLLiteral(t schemaSQLToken) bool {
	if t.unicode {
		return t.kind == 's' && t.err == nil
	}
	if t.kind == 'd' {
		tag := dollarQuoteRe.FindString(t.text)
		return len(t.text) >= 2*len(tag) && strings.HasSuffix(t.text, tag)
	}
	if t.kind == 's' {
		opening := 1
		if t.text[0] != '\'' {
			opening = 2
		}
		return len(t.text) > opening && strings.HasSuffix(t.text, "'")
	}
	return false
}

func encodeSQLLiteral(t schemaSQLToken, content string) string {
	if t.unicode {
		// Keep U& semantics, including PostgreSQL's rejection when
		// standard_conforming_strings is off. Use the default escape and
		// consume the original UESCAPE clause while retaining its trivia.
		return t.text[:2] + "'" + strings.ReplaceAll(strings.ReplaceAll(content, `\`, `\\`), "'", "''") + "'" + t.trivia
	}
	if t.kind == 'd' {
		tag := dollarQuoteRe.FindString(t.text)
		// Decoding an inner Unicode literal can expose the body's delimiter.
		for strings.Contains(content, tag) {
			tag = strings.TrimSuffix(tag, "$") + "_$"
		}
		return tag + content + tag
	}
	if t.text[0] != '\'' {
		return t.text[:1] + "'" + strings.ReplaceAll(strings.ReplaceAll(content, `\`, `\\`), "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(content, "'", "''") + "'"
}

func significantTokens(tokens []schemaSQLToken) []int {
	var indexes []int
	for i, t := range tokens {
		if t.kind != 'w' && t.kind != 'c' {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

// remapDOBody rewrites only explicit relation-valued regclass literals.
// Other code stays unchanged because an identifier prefix may name a schema,
// table alias, or PL/pgSQL variable. Ordinary strings are never interpreted as SQL.
func remapDOBody(body, schema, temp string, objects map[string]bool) (string, error) {
	tokens := lexSQL(body)
	if err := unicodeTokenError(tokens); err != nil {
		return "", fmt.Errorf("decode DO body: %w", err)
	}
	significant := significantTokens(tokens)
	allowed := func(name string) bool { return objects == nil || objects[name] }
	for j := 0; j < len(significant); j++ {
		i := significant[j]
		t := tokens[i]
		if !completeSQLLiteral(t) {
			continue
		}
		next := j + 1
		regclass := false
		if next+2 < len(significant) && tokens[significant[next]].text == ":" && tokens[significant[next+1]].text == ":" {
			next += 2
			regclass = true
		} else if j >= 2 && tokenKeyword(tokens[significant[j-2]]) == "cast" && tokens[significant[j-1]].text == "(" && next < len(significant) && tokenKeyword(tokens[significant[next]]) == "as" {
			next++
			regclass = true
		}
		if !regclass || next >= len(significant) {
			continue
		}
		if tokenIdentifier(tokens[significant[next]]) == "pg_catalog" && next+2 < len(significant) && tokens[significant[next+1]].text == "." {
			next += 2
		}
		if tokenIdentifier(tokens[significant[next]]) != "regclass" {
			continue
		}
		content, err := literalContent(t)
		if err != nil {
			return "", fmt.Errorf("decode regclass literal in DO body: %w", err)
		}
		start := skipSpace(content, 0)
		name, end, ok := parseRegclassIdent(content, start)
		dot := skipSpace(content, end)
		if !ok || name != schema || dot >= len(content) || content[dot] != '.' {
			continue
		}
		object, next, ok := parseRegclassIdent(content, dot+1)
		if ok && skipSpace(content, next) == len(content) && allowed(object) {
			remapped := content[:start] + quoteIdent(temp) + content[end:]
			tokens[i].text = encodeSQLLiteral(t, remapped)
		}
	}
	var result strings.Builder
	for _, t := range tokens {
		result.WriteString(t.text)
	}
	return result.String(), nil
}

// Regclass input uses PostgreSQL's qualified-name grammar, not SQL tokens.
// In an unquoted component, characters such as &, / and embedded quotes are
// part of the name. Only a dot or whitespace ends the component.
func parseRegclassIdent(text string, start int) (string, int, bool) {
	start = skipSpace(text, start)
	if start == len(text) {
		return "", start, false
	}
	if text[start] == '"' {
		return parseQuotedIdent(text, start)
	}
	end := start
	for end < len(text) && text[end] != '.' && !strings.ContainsRune(" \t\n\r\f\v", rune(text[end])) {
		end++
	}
	name := []byte(text[start:end])
	// PostgreSQL folds ASCII letters in UTF-8 unquoted names.
	for i, c := range name {
		if c >= 'A' && c <= 'Z' {
			name[i] = c + ('a' - 'A')
		}
	}
	return string(name), end, end > start
}
