package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// generatedColumnIR builds a one-table IR whose column "doubled" is
// GENERATED ALWAYS AS (expr) STORED, with a plain index on it.
func generatedColumnIR(expr string) *ir.IR {
	table := &ir.Table{
		Schema: "public",
		Name:   "metrics",
		Type:   ir.TableTypeBase,
		Columns: []*ir.Column{
			{Name: "a", Position: 1, DataType: "integer", IsNullable: false},
			{Name: "doubled", Position: 2, DataType: "integer", IsNullable: true,
				IsGenerated: true, GeneratedKind: "s", GeneratedExpr: &expr},
		},
		Constraints: map[string]*ir.Constraint{},
		Indexes: map[string]*ir.Index{
			"metrics_doubled_idx": {
				Schema: "public", Table: "metrics", Name: "metrics_doubled_idx",
				Type: ir.IndexTypeRegular, Method: "btree",
				Columns: []*ir.IndexColumn{{Name: "doubled", Position: 1, Direction: "ASC"}},
			},
		},
	}
	return &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {Name: "public", Tables: map[string]*ir.Table{"metrics": table}},
		},
	}
}

func migrationSQL(diffs []Diff) string {
	var b strings.Builder
	for _, d := range diffs {
		for _, s := range d.Statements {
			b.WriteString(s.SQL)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// A generated-column expression change uses SET EXPRESSION AS on PostgreSQL
// 17+ (and when the version is unknown), and re-creates the column together
// with its dependent index on older servers, where that clause does not
// exist (issue #591).
func TestGeneratedExpressionChange_VersionGate(t *testing.T) {
	oldIR := generatedColumnIR("(a * 3)")
	newIR := generatedColumnIR("(a * 2)")

	for _, version := range []int{0, 17, 18} {
		got := migrationSQL(GenerateMigrationForTarget(oldIR, newIR, "public", version))
		want := "ALTER TABLE metrics ALTER COLUMN doubled SET EXPRESSION AS ((a * 2));\n"
		if got != want {
			t.Errorf("version %d: got\n%s\nwant\n%s", version, got, want)
		}
	}

	for _, version := range []int{14, 16} {
		got := migrationSQL(GenerateMigrationForTarget(oldIR, newIR, "public", version))
		want := strings.Join([]string{
			"ALTER TABLE metrics DROP COLUMN doubled;",
			"ALTER TABLE metrics ADD COLUMN doubled integer GENERATED ALWAYS AS ((a * 2)) STORED;",
			"CREATE INDEX IF NOT EXISTS metrics_doubled_idx ON metrics (doubled);",
			"",
		}, "\n")
		if got != want {
			t.Errorf("version %d: got\n%s\nwant\n%s", version, got, want)
		}
	}
}

func TestExprReferencesAnyColumn(t *testing.T) {
	cols := map[string]bool{"b": true, "my col": true, `a"b`: true}
	cases := []struct {
		expr string
		want bool
	}{
		{"(b + 1)", true},
		{"b", true},
		{"(\"my col\" * 2)", true},
		{`("a""b" + 1)`, true}, // embedded quote doubled by pg_get_expr
		{`("a"b" + 1)`, false},
		{"(b > 10)", true},
		{"(bb + 1)", false},
		{"(a + 1)", false},
		{"b(a)", false},        // function call, not a column
		{"('b'::text)", false}, // string literal
		{"", false},
	}
	for _, c := range cases {
		if got := exprReferencesAnyColumn(c.expr, cols); got != c.want {
			t.Errorf("exprReferencesAnyColumn(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}
