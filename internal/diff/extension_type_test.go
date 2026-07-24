package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// These tests cover the extension-owned-type schema-qualifier fix: when a
// column or type's data type is owned by a Postgres extension (e.g. pgvector's
// "vector"), the schema it resolves to during plan-time introspection can
// legitimately differ from the live DB's schema for the same extension
// (temp-schema environments can't always relocate an extension into a
// throwaway schema). A schema-only mismatch between two extension-owned sides
// must not be treated as a diff, while a genuine type change (e.g. a pgvector
// dimension change) still must be.

func TestColumnsEqual_ExtensionOwnedTypes(t *testing.T) {
	tests := []struct {
		name string
		old  *ir.Column
		new  *ir.Column
		want bool
	}{
		{
			name: "same extension, different schema qualifier -> equal",
			old:  &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
			new:  &ir.Column{Name: "embedding", DataType: "public.vector(384)", ExtensionName: "vector"},
			want: true,
		},
		{
			name: "same extension, same schema, no diff -> equal",
			old:  &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
			new:  &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
			want: true,
		},
		{
			name: "same extension, different schema, genuine dimension change -> not equal",
			old:  &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
			new:  &ir.Column{Name: "embedding", DataType: "public.vector(512)", ExtensionName: "vector"},
			want: false,
		},
		{
			name: "different extensions -> not equal",
			old:  &ir.Column{Name: "data", DataType: "public.hstore", ExtensionName: "hstore"},
			new:  &ir.Column{Name: "data", DataType: "utils.custom_ext_type", ExtensionName: "some_other_ext"},
			want: false,
		},
		{
			name: "one side extension-owned, other not -> falls through to normal compare, not equal",
			old:  &ir.Column{Name: "data", DataType: "utils.hstore", ExtensionName: "hstore"},
			new:  &ir.Column{Name: "data", DataType: "public.hstore", ExtensionName: ""},
			want: false,
		},
		{
			name: "neither extension-owned, schema mismatch outside target schema -> not equal (unchanged existing behavior)",
			old:  &ir.Column{Name: "status", DataType: "domain.order_status"},
			new:  &ir.Column{Name: "status", DataType: "public.order_status"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := columnsEqual(tt.old, tt.new, "domain")
			if got != tt.want {
				t.Errorf("columnsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateColumnSQL_ExtensionOwnedTypeChange(t *testing.T) {
	// A genuine dimension change on an extension-owned type must still emit
	// an ALTER COLUMN TYPE, with the schema qualifier stripped (it's a
	// plan-time artifact, not the live DB's actual state).
	cd := &ColumnDiff{
		Old: &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
		New: &ir.Column{Name: "embedding", DataType: "public.vector(512)", ExtensionName: "vector"},
	}

	statements := cd.generateColumnSQL("domain", "documents", "domain")

	found := false
	for _, stmt := range statements {
		if strings.Contains(stmt, "ALTER COLUMN embedding TYPE vector(512)") {
			found = true
			if strings.Contains(stmt, "public.vector") || strings.Contains(stmt, "domain.vector") {
				t.Errorf("generateColumnSQL() left a stray schema qualifier on an extension-owned type: %q", stmt)
			}
		}
	}
	if !found {
		t.Errorf("generateColumnSQL() = %v, want a statement setting the type to schema-less vector(512)", statements)
	}
}

func TestGenerateColumnSQL_ExtensionOwnedTypeNoDiff(t *testing.T) {
	// A schema-only mismatch on an extension-owned type must not emit any
	// ALTER COLUMN TYPE statement at all.
	cd := &ColumnDiff{
		Old: &ir.Column{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
		New: &ir.Column{Name: "embedding", DataType: "public.vector(384)", ExtensionName: "vector"},
	}

	statements := cd.generateColumnSQL("domain", "documents", "domain")

	for _, stmt := range statements {
		if strings.Contains(stmt, "ALTER COLUMN embedding TYPE") {
			t.Errorf("generateColumnSQL() emitted a spurious type-change statement: %q", stmt)
		}
	}
}

func TestTypesEqual_ExtensionOwnedCompositeAttribute(t *testing.T) {
	tests := []struct {
		name string
		old  *ir.Type
		new  *ir.Type
		want bool
	}{
		{
			name: "composite attribute: same extension, different schema qualifier -> equal",
			old: &ir.Type{Schema: "domain", Name: "search_result", Kind: ir.TypeKindComposite,
				Columns: []*ir.TypeColumn{{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"}}},
			new: &ir.Type{Schema: "domain", Name: "search_result", Kind: ir.TypeKindComposite,
				Columns: []*ir.TypeColumn{{Name: "embedding", DataType: "public.vector(384)", ExtensionName: "vector"}}},
			want: true,
		},
		{
			name: "composite attribute: same extension, genuine dimension change -> not equal",
			old: &ir.Type{Schema: "domain", Name: "search_result", Kind: ir.TypeKindComposite,
				Columns: []*ir.TypeColumn{{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"}}},
			new: &ir.Type{Schema: "domain", Name: "search_result", Kind: ir.TypeKindComposite,
				Columns: []*ir.TypeColumn{{Name: "embedding", DataType: "public.vector(512)", ExtensionName: "vector"}}},
			want: false,
		},
		{
			name: "domain base type: same extension, different schema qualifier -> equal",
			old: &ir.Type{Schema: "domain", Name: "embedding_type", Kind: ir.TypeKindDomain,
				BaseType: "domain.vector(384)", ExtensionName: "vector"},
			new: &ir.Type{Schema: "domain", Name: "embedding_type", Kind: ir.TypeKindDomain,
				BaseType: "public.vector(384)", ExtensionName: "vector"},
			want: true,
		},
		{
			name: "domain base type: same extension, genuine dimension change -> not equal",
			old: &ir.Type{Schema: "domain", Name: "embedding_type", Kind: ir.TypeKindDomain,
				BaseType: "domain.vector(384)", ExtensionName: "vector"},
			new: &ir.Type{Schema: "domain", Name: "embedding_type", Kind: ir.TypeKindDomain,
				BaseType: "public.vector(512)", ExtensionName: "vector"},
			want: false,
		},
		{
			name: "domain base type: neither extension-owned, schema mismatch outside target schema -> not equal (unchanged existing behavior)",
			old: &ir.Type{Schema: "domain", Name: "status_type", Kind: ir.TypeKindDomain,
				BaseType: "domain.order_status"},
			new: &ir.Type{Schema: "domain", Name: "status_type", Kind: ir.TypeKindDomain,
				BaseType: "public.order_status"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := typesEqual(tt.old, tt.new, "domain")
			if got != tt.want {
				t.Errorf("typesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripAnySchemaPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"unquoted schema", "domain.vector(384)", "vector(384)"},
		{"quoted schema", `"My Schema".vector(384)`, "vector(384)"},
		{"no schema prefix", "vector(384)", "vector(384)"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripAnySchemaPrefix(tt.input)
			if got != tt.want {
				t.Errorf("stripAnySchemaPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
