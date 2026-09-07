package include

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCopyLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    CopyDirective
		wantErr string
	}{
		{
			name: "columns and options",
			line: `\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)`,
			want: CopyDirective{Table: "country", Columns: "code, name", Path: "data/country.csv", Options: "WITH (FORMAT csv, HEADER)"},
		},
		{
			name: "no columns no options trailing semicolon",
			line: `  \copy country from 'data/country.tsv';`,
			want: CopyDirective{Table: "country", Path: "data/country.tsv"},
		},
		{
			name: "qualified quoted target",
			line: `\copy "public"."Country" (code) FROM 'x.csv' csv header`,
			want: CopyDirective{Schema: "public", Table: "Country", Columns: "code", Path: "x.csv", Options: "csv header"},
		},
		{
			name: "quote in path",
			line: `\copy t FROM 'it''s.csv' WITH (FORMAT csv)`,
			want: CopyDirective{Table: "t", Path: "it's.csv", Options: "WITH (FORMAT csv)"},
		},
		{
			name: "bare name is lowercased",
			line: `\copy Country FROM 'c.csv'`,
			want: CopyDirective{Table: "country", Path: "c.csv"},
		},
		{name: "to direction", line: `\copy t TO 'out.csv'`, wantErr: "TO is not supported"},
		{name: "stdin", line: `\copy t FROM stdin`, wantErr: "STDIN is not supported"},
		{name: "program", line: `\copy t FROM PROGRAM 'cat x'`, wantErr: "PROGRAM is not supported"},
		{name: "missing from", line: `\copy t 'x.csv'`, wantErr: "expected FROM"},
		{name: "unquoted path", line: `\copy t FROM data/x.csv`, wantErr: "quoted file path"},
		{name: "unterminated columns", line: `\copy t (a, b FROM 'x.csv'`, wantErr: "unterminated column list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok, err := parseCopyLine(tt.line)
			if !ok {
				t.Fatalf("parseCopyLine(%q) did not recognise a directive", tt.line)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *got != tt.want {
				t.Fatalf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestParseCopyLineIgnoresOtherLines(t *testing.T) {
	for _, line := range []string{
		`\i tables/x.sql`,
		`COPY country FROM stdin;`,
		`-- \copy country FROM 'x.csv'`,
		`SELECT 1;`,
	} {
		if _, ok, err := parseCopyLine(line); ok || err != nil {
			t.Errorf("parseCopyLine(%q) = ok %v, err %v; want a regular line", line, ok, err)
		}
	}
}

func TestCopyMarkerRoundTrip(t *testing.T) {
	d := &CopyDirective{Schema: "public", Table: "t", Columns: "a, b", Path: "/abs/data/t.csv", Options: `WITH (FORMAT csv, HEADER, NULL 'N/A')`}
	line := EncodeCopyMarker(d)
	if !strings.HasPrefix(line, "--") {
		t.Fatalf("marker must be a SQL comment line: %q", line)
	}
	got, ok := ParseCopyMarker(line)
	if !ok {
		t.Fatalf("ParseCopyMarker failed for %q", line)
	}
	if *got != *d {
		t.Fatalf("got %+v, want %+v", *got, *d)
	}
	if _, ok := ParseCopyMarker("-- plain comment"); ok {
		t.Fatal("plain comment parsed as marker")
	}
}

func TestProcessFileCopyDirectives(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("main.sql", "\\i tables/country.sql\n\\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)\n")
	mustWrite("tables/country.sql", "CREATE TABLE country (code text PRIMARY KEY, name text);\n\\copy country FROM 'rows.csv'\n")
	mustWrite("data/country.csv", "code,name\nUS,United States\n")
	mustWrite("tables/rows.csv", "XX\tNowhere\n")

	p := NewProcessor(dir)
	out, err := p.ProcessFile(filepath.Join(dir, "main.sql"))
	if err != nil {
		t.Fatalf("ProcessFile: %v", err)
	}

	directives := p.copies
	if len(directives) != 2 {
		t.Fatalf("got %d directives, want 2", len(directives))
	}
	// Paths resolve relative to the file containing the directive.
	if want := filepath.Join(dir, "tables", "rows.csv"); directives[0].Path != want {
		t.Errorf("nested directive path = %q, want %q", directives[0].Path, want)
	}
	if want := filepath.Join(dir, "data", "country.csv"); directives[1].Path != want {
		t.Errorf("main directive path = %q, want %q", directives[1].Path, want)
	}
	if tables := p.CopyTables(); len(tables) != 1 || tables[0] != "country" {
		t.Errorf("CopyTables = %v, want [country]", tables)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	markers := 0
	for _, line := range lines {
		if strings.Contains(line, `\copy`) {
			t.Errorf("directive left in output: %q", line)
		}
		if _, ok := ParseCopyMarker(line); ok {
			markers++
		}
	}
	if markers != 2 {
		t.Errorf("got %d marker lines, want 2", markers)
	}
	if !strings.Contains(out, "CREATE TABLE country") {
		t.Errorf("included SQL missing from output")
	}
}

func TestProcessFileCopyPathEscapesBaseDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.sql"), []byte("\\copy t FROM '../outside.csv'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := NewProcessor(dir).ProcessFile(filepath.Join(dir, "main.sql"))
	if err == nil || !strings.Contains(err.Error(), "traversal") {
		t.Fatalf("expected traversal error, got %v", err)
	}
}

func TestProcessFileCopyMissingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.sql"), []byte("\\copy t FROM 'data/missing.csv'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := NewProcessor(dir).ProcessFile(filepath.Join(dir, "main.sql"))
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing file error, got %v", err)
	}
}
