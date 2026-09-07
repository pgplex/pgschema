package include

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// CopyDirective is a parsed `\copy <table> [(cols)] FROM '<path>' [options]`
// line. The include processor replaces each directive with a marker line
// (see EncodeCopyMarker) that the desired-state provider turns into a
// server-side COPY ... FROM STDIN streamed from the file.
type CopyDirective struct {
	Schema  string `json:"schema,omitempty"`  // schema qualifier as written, empty if none
	Table   string `json:"table"`             // unquoted table name
	Columns string `json:"columns,omitempty"` // raw column list without parentheses, empty if none
	Path    string `json:"path"`              // absolute path of the data file
	Options string `json:"options,omitempty"` // everything after the path, passed to COPY verbatim
}

// copyMarkerPrefix starts a marker line. It is a SQL comment, so any text
// transform that is comment-aware leaves it alone, and it is specific enough
// not to occur in real schema files.
const copyMarkerPrefix = "--pgschema:copy "

var copyLineRegex = regexp.MustCompile(`^\s*\\copy\s+(.*?)\s*;?\s*$`)

// HasCopyMarkers reports whether processed SQL contains any marker line, so
// callers can skip line scanning for the common case of no reference data.
func HasCopyMarkers(sqlText string) bool {
	return strings.Contains(sqlText, copyMarkerPrefix)
}

// EncodeCopyMarker renders a directive as a marker line.
func EncodeCopyMarker(d *CopyDirective) string {
	b, _ := json.Marshal(d)
	return copyMarkerPrefix + string(b)
}

// ParseCopyMarker decodes a marker line. ok is false for any other line.
func ParseCopyMarker(line string) (d *CopyDirective, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), strings.TrimSpace(copyMarkerPrefix))
	if !found {
		return nil, false
	}
	var out CopyDirective
	if err := json.Unmarshal([]byte(strings.TrimSpace(rest)), &out); err != nil {
		return nil, false
	}
	return &out, true
}

// parseCopyLine parses a `\copy` line. ok is false for any other line. Path
// is returned as written.
func parseCopyLine(line string) (d *CopyDirective, ok bool, err error) {
	if !strings.Contains(line, `\copy`) {
		return nil, false, nil
	}
	m := copyLineRegex.FindStringSubmatch(line)
	if m == nil {
		return nil, false, nil
	}
	d, err = parseCopyDirective(m[1])
	return d, true, err
}

// parseCopyDirective parses the text after `\copy`.
func parseCopyDirective(rest string) (*CopyDirective, error) {
	d := &CopyDirective{}

	// Target: [schema.]table
	var err error
	var first string
	first, rest, err = readIdentifier(rest)
	if err != nil {
		return nil, err
	}
	rest = strings.TrimLeft(rest, " \t")
	if strings.HasPrefix(rest, ".") {
		d.Schema = first
		d.Table, rest, err = readIdentifier(rest[1:])
		if err != nil {
			return nil, err
		}
		rest = strings.TrimLeft(rest, " \t")
	} else {
		d.Table = first
	}

	// Optional column list
	if strings.HasPrefix(rest, "(") {
		end := strings.Index(rest, ")")
		if end < 0 {
			return nil, fmt.Errorf("unterminated column list in \\copy directive")
		}
		d.Columns = strings.TrimSpace(rest[1:end])
		rest = strings.TrimLeft(rest[end+1:], " \t")
	}

	// Direction keyword
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil, fmt.Errorf("\\copy directive is missing FROM")
	}
	switch strings.ToLower(fields[0]) {
	case "from":
	case "to":
		return nil, fmt.Errorf("\\copy ... TO is not supported; only \\copy ... FROM '<file>' loads reference data")
	default:
		return nil, fmt.Errorf("expected FROM in \\copy directive, found %q", fields[0])
	}
	rest = strings.TrimLeft(rest[len(fields[0]):], " \t")

	// Source: quoted path
	if !strings.HasPrefix(rest, "'") {
		word := rest
		if f := strings.Fields(rest); len(f) > 0 {
			word = f[0]
		}
		switch strings.ToLower(word) {
		case "stdin", "pstdin":
			return nil, fmt.Errorf("\\copy ... FROM STDIN is not supported; put the rows in a file and use FROM '<file>'")
		case "program":
			return nil, fmt.Errorf("\\copy ... FROM PROGRAM is not supported")
		}
		return nil, fmt.Errorf("\\copy source must be a quoted file path, found %q", word)
	}
	path, rest, err := readSingleQuoted(rest)
	if err != nil {
		return nil, err
	}
	d.Path = path
	d.Options = strings.TrimSpace(rest)
	return d, nil
}

// readIdentifier reads a bare or double-quoted SQL identifier from the start
// of s and returns its unquoted value and the remainder.
func readIdentifier(s string) (string, string, error) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", "", fmt.Errorf("\\copy directive is missing a table name")
	}
	if s[0] == '"' {
		var b strings.Builder
		i := 1
		for i < len(s) {
			if s[i] == '"' {
				if i+1 < len(s) && s[i+1] == '"' {
					b.WriteByte('"')
					i += 2
					continue
				}
				return b.String(), s[i+1:], nil
			}
			b.WriteByte(s[i])
			i++
		}
		return "", "", fmt.Errorf("unterminated quoted identifier in \\copy directive")
	}
	end := 0
	for end < len(s) {
		c := s[end]
		if c == '_' || c == '$' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80 {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return "", "", fmt.Errorf("invalid table name in \\copy directive: %q", s)
	}
	return strings.ToLower(s[:end]), s[end:], nil
}

// readSingleQuoted reads a single-quoted string (” escapes a quote) from the
// start of s and returns its value and the remainder.
func readSingleQuoted(s string) (string, string, error) {
	var b strings.Builder
	i := 1
	for i < len(s) {
		if s[i] == '\'' {
			if i+1 < len(s) && s[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			return b.String(), s[i+1:], nil
		}
		b.WriteByte(s[i])
		i++
	}
	return "", "", fmt.Errorf("unterminated file path in \\copy directive")
}
