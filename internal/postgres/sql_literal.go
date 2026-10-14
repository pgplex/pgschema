package postgres

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// decodeUnicodeEscape requires the exact PostgreSQL escape width. Surrogate
// code units are returned to the caller, which must validate and combine pairs.
func decodeUnicodeEscape(text string, start int) (rune, int, error) {
	width := 4
	if text[start] == 'U' {
		width = 8
	}
	end := start + 1 + width
	if end > len(text) {
		return 0, 0, fmt.Errorf("incomplete Unicode escape at byte %d", start)
	}
	value, err := strconv.ParseUint(text[start+1:end], 16, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid Unicode escape at byte %d: %w", start, err)
	}
	if value == 0 || value > utf8.MaxRune {
		return 0, 0, fmt.Errorf("invalid Unicode value U+%X at byte %d", value, start)
	}
	return rune(value), end, nil
}

// literalContent decodes PostgreSQL dollar, standard, and E-string literals.
// Invalid escape literals fail at the remapping boundary before SQL execution.
func literalContent(t schemaSQLToken) (string, error) {
	if t.unicode {
		return t.content, t.err
	}
	if t.kind == 'd' {
		tag := dollarQuoteRe.FindString(t.text)
		return t.text[len(tag) : len(t.text)-len(tag)], nil
	}
	text := t.text
	escape := text[0] != '\''
	if escape {
		text = text[1:]
	}
	text = text[1 : len(text)-1]
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\'' && i+1 < len(text) && text[i+1] == '\'' {
			b.WriteByte('\'')
			i++
			continue
		}
		if !escape || text[i] != '\\' {
			b.WriteByte(text[i])
			continue
		}
		if i+1 == len(text) {
			return "", fmt.Errorf("incomplete escape at byte %d", i)
		}
		i++
		switch text[i] {
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u', 'U':
			value, end, err := decodeUnicodeEscape(text, i)
			if err != nil {
				return "", err
			}
			if value >= 0xD800 && value <= 0xDBFF {
				if end+1 >= len(text) || text[end] != '\\' || (text[end+1] != 'u' && text[end+1] != 'U') {
					return "", fmt.Errorf("missing low Unicode surrogate at byte %d", end)
				}
				low, next, err := decodeUnicodeEscape(text, end+1)
				if err != nil {
					return "", err
				}
				if low < 0xDC00 || low > 0xDFFF {
					return "", fmt.Errorf("invalid low Unicode surrogate U+%X at byte %d", low, end)
				}
				value = utf16.DecodeRune(value, low)
				end = next
			} else if value >= 0xDC00 && value <= 0xDFFF {
				return "", fmt.Errorf("unexpected low Unicode surrogate at byte %d", i)
			}
			b.WriteRune(value)
			i = end - 1
		case 'x':
			start, end := i+1, i+1
			for end < len(text) && end-start < 2 && strings.ContainsRune("0123456789abcdefABCDEF", rune(text[end])) {
				end++
			}
			if end == start {
				b.WriteByte('x')
				continue
			}
			value, err := strconv.ParseUint(text[start:end], 16, 8)
			if err != nil {
				return "", fmt.Errorf("invalid hexadecimal escape at byte %d: %w", i, err)
			}
			b.WriteByte(byte(value))
			i = end - 1
		default:
			if text[i] >= '0' && text[i] <= '7' {
				start, end := i, i+1
				for end < len(text) && end-start < 3 && text[end] >= '0' && text[end] <= '7' {
					end++
				}
				value, err := strconv.ParseUint(text[start:end], 8, 32)
				if err != nil {
					return "", fmt.Errorf("invalid octal escape at byte %d: %w", i, err)
				}
				b.WriteByte(byte(value))
				i = end - 1
			} else {
				b.WriteByte(text[i])
			}
		}
	}
	content := b.String()
	if strings.ContainsRune(content, 0) {
		return "", fmt.Errorf("SQL literals cannot contain a zero byte")
	}
	return content, nil
}
