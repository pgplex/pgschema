package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// TestReferencesNewFunctionQuotedIdentifiers covers issue #571: functionCallRegex
// previously only matched unquoted identifiers, so a case-sensitive, double-quoted
// function name (and one containing an escaped "" quote) was never recognized as
// a dependency. It also covers the follow-up review feedback that quoted and
// unquoted references must not be conflated: PostgreSQL only folds *unquoted*
// identifiers to lowercase, so "myfunc"() and "MyFunc"() are distinct functions.
func TestReferencesNewFunctionQuotedIdentifiers(t *testing.T) {
	tests := []struct {
		name          string
		expr          string
		defaultSchema string
		newFunctions  map[string]struct{}
		want          bool
	}{
		{
			name:          "unquoted call",
			expr:          "myfunc()",
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "myfunc"): {}},
			want:          true,
		},
		{
			name:          "quoted case-sensitive call",
			expr:          `"MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "quoted schema and quoted function",
			expr:          `"MySchema"."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("MySchema", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "unquoted schema, quoted function",
			expr:          `myschema."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("myschema", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "quoted identifier with escaped quote",
			expr:          `"My""Func"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", `My"Func`): {}},
			want:          true,
		},
		{
			name:          "quoted call to unrelated function",
			expr:          `"OtherFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          false,
		},
		{
			name:          "unquoted call does not match a quoted mixed-case function of the same letters",
			expr:          "myfunc()",
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          false,
		},
		{
			name:          "quoted call does not match an unquoted lowercase function of the same letters",
			expr:          `"MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "myfunc"): {}},
			want:          false,
		},
		{
			name:          "quoted schema containing a literal dot does not collide with a different qualified split",
			expr:          `"a.b".c()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("a", "b.c"): {}},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := referencesNewFunction(tt.expr, tt.defaultSchema, tt.newFunctions)
			if got != tt.want {
				t.Errorf("referencesNewFunction(%q, %q) = %v, want %v", tt.expr, tt.defaultSchema, got, tt.want)
			}
		})
	}
}

func TestNormalizeFunctionIdentifier(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`myfunc`, "myfunc"},
		{`"MyFunc"`, "MyFunc"},
		{`"MySchema"."MyFunc"`, functionGraphKey("MySchema", "MyFunc")},
		{`myschema."MyFunc"`, functionGraphKey("myschema", "MyFunc")},
		{`"My""Func"`, `My"Func`},
		{`"a.b".c`, functionGraphKey("a.b", "c")},
		{`a."b.c"`, functionGraphKey("a", "b.c")},
		{`MYFUNC`, "myfunc"},
		{`MySchema.MYFUNC`, functionGraphKey("myschema", "myfunc")},
		{`other . "Helper"`, functionGraphKey("other", "Helper")},
		{`"MySchema"  .  MyFunc`, functionGraphKey("MySchema", "myfunc")},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := normalizeFunctionIdentifier(tt.raw)
			if got != tt.want {
				t.Errorf("normalizeFunctionIdentifier(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNormalizeFunctionIdentifierNoCollision(t *testing.T) {
	// A plain "."-joined key would flatten both of these to "a.b.c"; the
	// NUL-separated functionGraphKey must keep them distinct.
	quotedSchema := normalizeFunctionIdentifier(`"a.b".c`)
	quotedName := normalizeFunctionIdentifier(`a."b.c"`)
	if quotedSchema == quotedName {
		t.Errorf("normalizeFunctionIdentifier(%q) and (%q) collided: both produced %q", `"a.b".c`, `a."b.c"`, quotedSchema)
	}
}

// TestTableReferencesNewFunctionIndexes covers the follow-up review feedback that a
// table's index (not just its columns/constraints) can depend on a new function: an
// expression/functional index column, or a partial index's WHERE predicate. Indexes
// are emitted immediately with their table, so missing this would let a CREATE INDEX
// run before the function it calls exists.
func TestTableReferencesNewFunctionIndexes(t *testing.T) {
	newFunctions := map[string]struct{}{functionGraphKey("public", "MyFunc"): {}}

	tests := []struct {
		name  string
		table *ir.Table
		want  bool
	}{
		{
			name: "expression index column calls new function",
			table: &ir.Table{
				Schema: "public",
				Indexes: map[string]*ir.Index{
					"idx_expr": {
						IsExpression: true,
						Columns:      []*ir.IndexColumn{{Name: `"MyFunc"(id)`, Position: 1}},
					},
				},
			},
			want: true,
		},
		{
			name: "partial index predicate calls new function",
			table: &ir.Table{
				Schema: "public",
				Indexes: map[string]*ir.Index{
					"idx_partial": {
						IsPartial: true,
						Columns:   []*ir.IndexColumn{{Name: "id", Position: 1}},
						Where:     `"MyFunc"() > 0`,
					},
				},
			},
			want: true,
		},
		{
			name: "exclusion constraint element calls new function",
			table: &ir.Table{
				Schema: "public",
				Constraints: map[string]*ir.Constraint{
					"widgets_excl": {
						Type:                ir.ConstraintTypeExclusion,
						ExclusionDefinition: `EXCLUDE USING btree ("MyFunc"(code) WITH =)`,
					},
				},
			},
			want: true,
		},
		{
			name: "partition key expression calls new function",
			table: &ir.Table{
				Schema:            "public",
				IsPartitioned:     true,
				PartitionStrategy: "RANGE",
				PartitionKey:      `"MyFunc"(code)`,
			},
			want: true,
		},
		{
			name: "plain index on unrelated column",
			table: &ir.Table{
				Schema: "public",
				Indexes: map[string]*ir.Index{
					"idx_plain": {
						Columns: []*ir.IndexColumn{{Name: "id", Position: 1}},
					},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tableReferencesNewFunction(tt.table, newFunctions)
			if got != tt.want {
				t.Errorf("tableReferencesNewFunction(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestFunctionLookupKeyPart(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"myfunc", "myfunc"},
		{"public", "public"},
		{"MyFunc", "MyFunc"},
		{"MySchema", "MySchema"},
		{`My"Func`, `My"Func`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := functionLookupKeyPart(tt.name)
			if got != tt.want {
				t.Errorf("functionLookupKeyPart(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
