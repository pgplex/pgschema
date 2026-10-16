package postgres

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// tokenKeyword excludes quoted identifiers even when their names match keywords.
func tokenKeyword(t schemaSQLToken) string {
	if t.kind != 'i' || t.unicode || strings.HasPrefix(t.text, `"`) {
		return ""
	}
	return strings.ToLower(t.text)
}

func unicodeTokenError(tokens []schemaSQLToken) error {
	for _, t := range tokens {
		if t.err != nil {
			return t.err
		}
	}
	return nil
}

// decodeUnicodeTokens consumes UESCAPE with its token so a rewrite cannot leave
// an orphaned clause. Unchanged tokens retain their complete original spelling.
func decodeUnicodeTokens(tokens []schemaSQLToken) []schemaSQLToken {
	var result []schemaSQLToken
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if len(t.text) < 3 || (t.text[0] != 'u' && t.text[0] != 'U') || t.text[1] != '&' || (t.text[2] != '\'' && t.text[2] != '"') {
			result = append(result, t)
			continue
		}
		t.unicode = true
		if t.kind == '!' {
			t.err = fmt.Errorf("unterminated Unicode token")
			result = append(result, t)
			continue
		}
		quote := rune(t.text[2])
		payload := t.text[3 : len(t.text)-1]
		// PostgreSQL joins the raw fragments before interpreting Unicode escapes.
		// This also permits an escape or surrogate pair to span fragments.
		if t.kind == 's' {
			for {
				next := sqlStringContinuation(tokens, i)
				if next == i {
					break
				}
				payload += tokens[next].text[1 : len(tokens[next].text)-1]
				for n := i + 1; n <= next; n++ {
					t.text += tokens[n].text
					if n != next {
						t.trivia += tokens[n].text
					}
				}
				i = next
			}
		}
		escape := rune('\\')
		j := i + 1
		for j < len(tokens) && (tokens[j].kind == 'w' || tokens[j].kind == 'c') {
			j++
		}
		if j < len(tokens) && tokenKeyword(tokens[j]) == "uescape" {
			k := j + 1
			for k < len(tokens) && (tokens[k].kind == 'w' || tokens[k].kind == 'c') {
				k++
			}
			if k == len(tokens) || !completeSQLLiteral(tokens[k]) || strings.HasPrefix(strings.ToLower(tokens[k].text), "u&") {
				t.err = fmt.Errorf("UESCAPE requires a simple string literal")
			} else {
				value, err := literalContent(tokens[k])
				escaped := tokens[k].kind == 's' && tokens[k].text[0] != '\''
				if tokens[k].kind == 's' {
					for {
						next := sqlStringContinuation(tokens, k)
						if next == k {
							break
						}
						fragment := tokens[next]
						if escaped {
							fragment.text = "E" + fragment.text
						}
						part, partErr := literalContent(fragment)
						value += part
						if err == nil {
							err = partErr
						}
						k = next
					}
				}
				if err != nil {
					t.err = err
				} else {
					r, size := utf8.DecodeRuneInString(value)
					if !utf8.ValidString(value) || len(value) != 1 || size != 1 || strings.ContainsRune("0123456789abcdefABCDEF+'\" \t\n\r\f\v", r) {
						t.err = fmt.Errorf("invalid UESCAPE character %q", value)
					} else {
						escape = r
					}
				}
				for n := i + 1; n <= k; n++ {
					t.text += tokens[n].text
					if tokens[n].kind == 'w' || tokens[n].kind == 'c' {
						t.trivia += tokens[n].text
					}
				}
				i = k
			}
		}
		if !utf8.ValidString(t.text) {
			t.err = fmt.Errorf("invalid UTF-8 in Unicode token")
		}
		if t.err == nil {
			t.content, t.err = decodeSQLUnicode(payload, quote, escape)
		}
		result = append(result, t)
	}
	return result
}

// sqlStringContinuation follows PostgreSQL quote continuation: a newline is
// required, line comments are allowed, and block comments stop continuation.
// Only an unprefixed single-quoted fragment can continue a string.
func sqlStringContinuation(tokens []schemaSQLToken, start int) int {
	newline := false
	k := start + 1
	for k < len(tokens) {
		t := tokens[k]
		if t.kind != 'w' && !(t.kind == 'c' && strings.HasPrefix(t.text, "--")) {
			break
		}
		newline = newline || strings.ContainsAny(t.text, "\n\r")
		k++
	}
	if newline && k < len(tokens) && tokens[k].kind == 's' && tokens[k].text[0] == '\'' && completeSQLLiteral(tokens[k]) {
		return k
	}
	return start
}

// decodeSQLUnicode implements PostgreSQL U& escapes, including adjacent UTF-16
// surrogate pairs. SQL quote doubling is independent of the escape character.
func decodeSQLUnicode(text string, quote, escape rune) (string, error) {
	if !utf8.ValidString(text) {
		return "", fmt.Errorf("invalid UTF-8 in Unicode token")
	}
	var b strings.Builder
	pending := rune(0)
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		escaped := false
		if r == quote && strings.HasPrefix(text[i:], string(quote)) {
			i += utf8.RuneLen(quote)
		} else if r == escape {
			if strings.HasPrefix(text[i:], string(escape)) {
				i += utf8.RuneLen(escape)
			} else {
				width := 4
				if i < len(text) && text[i] == '+' {
					width = 6
					i++
				}
				if i+width > len(text) {
					return "", fmt.Errorf("incomplete Unicode escape")
				}
				digits := text[i : i+width]
				for _, c := range digits {
					if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
						return "", fmt.Errorf("invalid Unicode escape %q", digits)
					}
				}
				value, err := strconv.ParseUint(digits, 16, 32)
				if err != nil {
					return "", err
				}
				if value == 0 || value > utf8.MaxRune {
					return "", fmt.Errorf("invalid Unicode value U+%X", value)
				}
				r = rune(value)
				i += width
				escaped = true
			}
		}
		if pending != 0 {
			if !escaped || r < 0xDC00 || r > 0xDFFF {
				return "", fmt.Errorf("invalid low Unicode surrogate")
			}
			r = utf16.DecodeRune(pending, r)
			pending = 0
		} else if escaped && r >= 0xD800 && r <= 0xDBFF {
			pending = r
			continue
		} else if escaped && r >= 0xDC00 && r <= 0xDFFF {
			return "", fmt.Errorf("unexpected low Unicode surrogate")
		}
		b.WriteRune(r)
	}
	if pending != 0 {
		return "", fmt.Errorf("missing low Unicode surrogate")
	}
	return b.String(), nil
}
