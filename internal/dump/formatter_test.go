package dump

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/ir"
)

// TestGetCommentSchemaName_QualifySchema covers the comment-header schema label,
// which is observable output for `dump --qualify-schema`. Without the flag the
// target schema collapses to "-" (pg_dump style); with the flag it is preserved
// so the header matches the schema-qualified DDL. Cross-schema paths always keep
// their schema; unqualified paths are returned unchanged.
func TestGetCommentSchemaName_QualifySchema(t *testing.T) {
	tests := []struct {
		name          string
		targetSchema  string
		qualifySchema bool
		path          string
		want          string
	}{
		{
			name:          "target schema collapses to dash by default",
			targetSchema:  "public",
			qualifySchema: false,
			path:          "public.users",
			want:          "-",
		},
		{
			name:          "target schema is preserved under forced qualification",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "public.users",
			want:          "public",
		},
		{
			name:          "cross-schema path keeps its schema (default)",
			targetSchema:  "public",
			qualifySchema: false,
			path:          "other.users",
			want:          "other",
		},
		{
			name:          "cross-schema path keeps its schema (forced)",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "other.users",
			want:          "other",
		},
		{
			name:          "unqualified path is returned unchanged",
			targetSchema:  "public",
			qualifySchema: true,
			path:          "users",
			want:          "users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDumpFormatter("PostgreSQL 17.5", tt.targetSchema, false, tt.qualifySchema)
			if got := f.getCommentSchemaName(tt.path); got != tt.want {
				t.Errorf("getCommentSchemaName(%q) with qualifySchema=%v = %q, want %q",
					tt.path, tt.qualifySchema, got, tt.want)
			}
		})
	}
}

func TestFormatMultiFilePostponedConstraints(t *testing.T) {
	constraint := func(table, name string) diff.Diff {
		return diff.Diff{
			Type: diff.DiffTypeTableConstraint, Operation: diff.DiffOperationCreate,
			Path:       "public." + table + "." + name,
			Source:     &ir.Constraint{Schema: "public", Table: table, Name: name, Type: ir.ConstraintTypeForeignKey},
			Statements: []diff.SQLStatement{{SQL: fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (id) REFERENCES b (id);", ir.QuoteIdentifier(table), ir.QuoteIdentifier(name))}},
		}
	}
	diffs := []diff.Diff{
		{Type: diff.DiffTypeTable, Path: "public.a", Source: &ir.Table{Name: "a"}, Statements: []diff.SQLStatement{{SQL: "CREATE TABLE a (id int PRIMARY KEY);"}}},
		{Type: diff.DiffTypeTable, Path: "public.b", Source: &ir.Table{Name: "b"}, Statements: []diff.SQLStatement{{SQL: "CREATE TABLE b (id int PRIMARY KEY);"}}},
		constraint("a", "shared_fk"),
		// An intervening object must stay between postponed constraints.
		{Type: diff.DiffTypeView, Path: "public.v", Source: &ir.View{Name: "v"}, Statements: []diff.SQLStatement{{SQL: "CREATE VIEW v AS SELECT * FROM a;"}}},
		constraint("b", "shared_fk"),
		constraint("a", "other_fk"),
		// These table names sanitize to the same filename. The sequence number
		// must keep the constraint files distinct, even for quoted identifiers.
		constraint("a.b", "quoted.fk"),
		constraint("a b", "quoted fk"),
		{Type: diff.DiffTypeTableComment, Path: "public.a", Source: &ir.Table{Name: "a"}, Statements: []diff.SQLStatement{{SQL: "COMMENT ON TABLE a IS 'kept';"}}},
	}
	wantFiles := []string{
		"tables/a.sql", "tables/b.sql", "constraints/0001_a.sql", "views/v.sql",
		"constraints/0002_b.sql", "constraints/0003_a.sql",
		"constraints/0004_a_b.sql", "constraints/0005_a_b.sql",
	}
	for _, noComments := range []bool{false, true} {
		for _, qualifySchema := range []bool{false, true} {
			t.Run(fmt.Sprintf("noComments=%v/qualifySchema=%v", noComments, qualifySchema), func(t *testing.T) {
				formatter := NewDumpFormatter("PostgreSQL 18", "public", noComments, qualifySchema)
				var first map[string]string
				for run := 0; run < 3; run++ {
					dir := t.TempDir()
					path := filepath.Join(dir, "schema.sql")
					if err := formatter.FormatMultiFile(diffs, path); err != nil {
						t.Fatal(err)
					}
					read := func(name string) string {
						t.Helper()
						content, err := os.ReadFile(filepath.Join(dir, name))
						if err != nil {
							t.Fatal(err)
						}
						return string(content)
					}
					files := map[string]string{"schema.sql": read("schema.sql")}
					wantIncludes := "\\i " + strings.Join(wantFiles, "\n\\i ") + "\n"
					if !strings.HasSuffix(files["schema.sql"], wantIncludes) {
						t.Fatalf("Unexpected include order:\n%s", files["schema.sql"])
					}
					for _, name := range wantFiles {
						files[name] = read(name)
					}
					if strings.Contains(files["tables/a.sql"], "FOREIGN KEY") || strings.Contains(files["tables/b.sql"], "FOREIGN KEY") {
						t.Fatal("Postponed constraints were bundled with a table")
					}
					if !strings.Contains(files["tables/a.sql"], "COMMENT ON TABLE a IS 'kept';") {
						t.Fatal("Table comment was lost")
					}
					for _, index := range []int{2, 4, 5, 6, 7} {
						content := files[wantFiles[index]]
						step := diffs[index]
						if !strings.HasSuffix(content, step.Statements[0].SQL+"\n") {
							t.Fatalf("Constraint SQL changed in %s: %s", wantFiles[index], content)
						}
						if strings.Contains(content, "-- Name:") == noComments {
							t.Fatalf("Incorrect comment header in %s", wantFiles[index])
						}
						if !noComments {
							schema := "-"
							if qualifySchema {
								schema = "public"
							}
							if !strings.Contains(content, "Type: CONSTRAINT; Schema: "+schema+"; Owner: -") {
								t.Fatalf("Incorrect schema header: %s", content)
							}
						}
					}
					if run == 0 {
						first = files
					} else {
						for name, content := range files {
							if content != first[name] {
								t.Fatalf("Non-deterministic output in %s", name)
							}
						}
					}
				}
			})
		}
	}
}

func TestFormatMultiFilePoliciesAfterNamedKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.sql")
	diffs := []diff.Diff{
		{Type: diff.DiffTypeTable, Path: "public.events", Source: &ir.Table{Name: "events"}, Statements: []diff.SQLStatement{{SQL: "CREATE TABLE events (id int NOT NULL, a int);"}}},
		{Type: diff.DiffTypeTableConstraint, Path: "public.events.pk", Source: &ir.Constraint{Table: "events", Name: "pk", Type: ir.ConstraintTypePrimaryKey}, Statements: []diff.SQLStatement{{SQL: "ALTER TABLE ONLY events ADD CONSTRAINT pk PRIMARY KEY (id);"}}},
		{Type: diff.DiffTypeTableIndex, Operation: diff.DiffOperationAttach, Path: "public.events.pk", Source: &ir.Index{Table: "events", Name: "pk", IsPartitioned: true}, Statements: []diff.SQLStatement{{SQL: "ALTER INDEX pk ATTACH PARTITION leaf_pk;"}}},
		{Type: diff.DiffTypeTableComment, Path: "public.events", Source: &ir.Constraint{Table: "events", Name: "pk", Type: ir.ConstraintTypePrimaryKey}, Statements: []diff.SQLStatement{{SQL: "COMMENT ON CONSTRAINT pk ON events IS 'key';"}}},
		{Type: diff.DiffTypeTablePolicy, Path: "public.events.grouped", Source: &ir.RLSPolicy{Table: "events", Name: "grouped"}, Statements: []diff.SQLStatement{{SQL: "CREATE POLICY grouped ON events USING (id IN (SELECT id FROM events GROUP BY id HAVING a IS NOT NULL));"}}},
	}
	if err := NewDumpFormatter("PostgreSQL 18", "public", false, false).FormatMultiFile(diffs, path); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "\\i tables/events.sql\n\\i constraints/0001_events.sql\n\\i indexes/0001_pk.sql\n\\i policies/0001_events.sql\n"
	if !strings.HasSuffix(string(content), want) {
		t.Fatalf("unexpected include order: %s", content)
	}
	table, err := os.ReadFile(filepath.Join(filepath.Dir(path), "tables", "events.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(table), "CREATE POLICY") {
		t.Fatal("policy moved before its key")
	}
	constraint, err := os.ReadFile(filepath.Join(filepath.Dir(path), "constraints", "0001_events.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(constraint), "ADD CONSTRAINT pk PRIMARY KEY (id);\n\nCOMMENT ON CONSTRAINT pk ON events IS 'key';") {
		t.Fatalf("constraint comment lost its creation order: %s", constraint)
	}
}
