package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestDisplayKey(t *testing.T) {
	tests := map[string]string{
		"US":                            "US",
		"a" + keySeparator + "b":        "a,b",
		"a,b" + keySeparator + "c":      `"a,b",c`,
		"a" + keySeparator + "b,c":      `a,"b,c"`,
		`say "hi"` + keySeparator + "x": `"say ""hi""",x`,
		"" + keySeparator + "":          ",",
	}
	for key, want := range tests {
		if got := displayKey(key); got != want {
			t.Errorf("displayKey(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestSecondaryUniqueKeysMergesNullSemantics(t *testing.T) {
	table := &ir.Table{
		Constraints: map[string]*ir.Constraint{
			"pk": {Type: ir.ConstraintTypePrimaryKey, Columns: []*ir.ConstraintColumn{{Name: "id"}}},
			"uq": {Type: ir.ConstraintTypeUnique, Columns: []*ir.ConstraintColumn{{Name: "code"}, {Name: "region"}}},
		},
		Indexes: map[string]*ir.Index{
			"idx": {Type: ir.IndexTypeUnique, NullsNotDistinct: true, Columns: []*ir.IndexColumn{{Name: "code"}, {Name: "region"}}},
			"pki": {Type: ir.IndexTypeUnique, Columns: []*ir.IndexColumn{{Name: "id"}}},
		},
	}
	keys := secondaryUniqueKeys(table, []string{"id"})
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1 (primary key excluded, duplicates merged): %+v", len(keys), keys)
	}
	if !keys[0].nullsNotDistinct {
		t.Errorf("expected NULLS NOT DISTINCT to win when any definition on the columns requires it")
	}
	if got := strings.Join(keys[0].columns, ","); got != "code,region" {
		t.Errorf("columns = %q, want code,region", got)
	}
}
