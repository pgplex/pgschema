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

// These tests cover the DDL-emission half of the fix: for CREATE-path
// statements (ADD COLUMN, CREATE TABLE, CREATE TYPE) there is no "old" side
// of *this* column/type to compare against, so bare-stripping the schema
// unconditionally is unsafe - it silently assumes the extension either lives
// in the target schema or public, which is false whenever an extension is
// deliberately installed elsewhere (e.g. hstore installed once into a shared
// "utils" schema, per the real add_column_cross_schema_custom_type fixture).
// resolveExtensionTypeSchema instead qualifies with wherever oldIR shows the
// extension is genuinely installed, falling back to bare only when the
// extension isn't used anywhere yet in oldIR.

func TestSchemaPrefixOf(t *testing.T) {
	tests := []struct{ in, want string }{
		{"domain.vector(384)", "domain"},
		{`"My Schema".vector`, "My Schema"},
		{"vector(384)", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := schemaPrefixOf(tt.in); got != tt.want {
			t.Errorf("schemaPrefixOf(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildExtensionSchemaMap(t *testing.T) {
	oldIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {
						Schema: "public", Name: "users",
						Columns: []*ir.Column{
							{Name: "id", DataType: "integer"},
							{Name: "metadata", DataType: "utils.hstore", ExtensionName: "hstore"},
						},
					},
				},
				Types: map[string]*ir.Type{
					"embedding_type": {
						Schema: "public", Name: "embedding_type", Kind: ir.TypeKindDomain,
						BaseType: "domain.vector(384)", ExtensionName: "vector",
					},
					"search_result": {
						Schema: "public", Name: "search_result", Kind: ir.TypeKindComposite,
						Columns: []*ir.TypeColumn{
							{Name: "embedding", DataType: "domain.vector(384)", ExtensionName: "vector"},
						},
					},
				},
			},
		},
	}

	got := buildExtensionSchemaMap(oldIR)
	if got["hstore"] != "utils" {
		t.Errorf("expected hstore -> utils, got %q", got["hstore"])
	}
	if got["vector"] != "domain" {
		t.Errorf("expected vector -> domain (from either the domain or composite type), got %q", got["vector"])
	}
	if _, exists := got["citext"]; exists {
		t.Errorf("expected no entry for an extension not used anywhere in oldIR")
	}
	if got := buildExtensionSchemaMap(nil); len(got) != 0 {
		t.Errorf("expected empty map for nil oldIR, got %v", got)
	}
}

// oldIR.Extensions (sourced directly from pg_extension) is authoritative and
// must be used even when no column/type anywhere yet uses that extension -
// the real gap that column/type inference alone cannot cover (e.g. an
// extension installed via a setup script but not yet referenced by any
// existing column at the time a migration first uses it).
func TestBuildExtensionSchemaMap_PrefersAuthoritativeExtensionsField(t *testing.T) {
	oldIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name:   "public",
				Tables: map[string]*ir.Table{},
				Types:  map[string]*ir.Type{},
			},
		},
		Extensions: map[string]string{
			"hstore": "utils",
		},
	}

	got := buildExtensionSchemaMap(oldIR)
	if got["hstore"] != "utils" {
		t.Errorf("expected hstore -> utils from the authoritative Extensions field even with no column usage, got %q", got["hstore"])
	}
}

func TestResolveExtensionTypeSchema(t *testing.T) {
	extensionSchemas := map[string]string{"hstore": "utils", "citext": "public"}

	tests := []struct {
		name                    string
		typeName, extensionName string
		targetSchema            string
		qualifySchema           bool
		want                    string
	}{
		{"known extension in a schema other than target -> qualifies with real schema",
			"public.hstore", "hstore", "public", false, "utils.hstore"},
		{"known extension, bare input -> qualifies with real schema",
			"hstore", "hstore", "public", false, "utils.hstore"},
		{"known extension already correctly qualified -> unchanged",
			"utils.hstore", "hstore", "public", false, "utils.hstore"},
		{"known extension installed IN the target schema -> smart-qualification omits the prefix",
			"public.citext", "citext", "public", false, "citext"},
		{"known extension installed in the target schema, but qualifySchema forces it back",
			"public.citext", "citext", "public", true, "public.citext"},
		{"unknown extension (not used anywhere in oldIR) -> falls back to bare",
			"public.vector", "vector", "public", false, "vector"},
		{"unknown extension, qualifySchema forced -> preserves original qualifier rather than dropping it (dump --qualify-schema contract)",
			"public.vector", "vector", "public", true, "public.vector"},
		{"unknown extension, qualifySchema forced, no original qualifier -> stays bare",
			"vector", "vector", "public", true, "vector"},
		{"empty type -> empty",
			"", "hstore", "public", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveExtensionTypeSchema(tt.typeName, tt.extensionName, extensionSchemas, tt.targetSchema, tt.qualifySchema); got != tt.want {
				t.Errorf("resolveExtensionTypeSchema(%q, %q, ..., %q, %v) = %q, want %q", tt.typeName, tt.extensionName, tt.targetSchema, tt.qualifySchema, got, tt.want)
			}
		})
	}
}

// End-to-end: adding a new column of an extension-owned type whose extension
// is genuinely installed outside both the target schema and "public" must
// qualify with that real schema, not strip to a bare reference that only
// resolves via search_path when the extension happens to live in the target
// schema or public - the real regression this test guards against.
func TestGenerateMigration_AddColumnExtensionOwnedTypeInNonTargetSchema_InferredFromSiblingColumn(t *testing.T) {
	oldIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {
						Schema: "public", Name: "users",
						Columns: []*ir.Column{{Name: "id", DataType: "integer", Position: 1}},
					},
					// Another table already uses hstore, so oldIR records where it
					// really lives.
					"other": {
						Schema: "public", Name: "other",
						Columns: []*ir.Column{
							{Name: "id", DataType: "integer", Position: 1},
							{Name: "tags", DataType: "utils.hstore", ExtensionName: "hstore", Position: 2},
						},
					},
				},
				Types: map[string]*ir.Type{},
			},
		},
	}
	newIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {
						Schema: "public", Name: "users",
						Columns: []*ir.Column{
							{Name: "id", DataType: "integer", Position: 1},
							{Name: "metadata", DataType: "public.hstore", ExtensionName: "hstore", Position: 2},
						},
					},
					"other": {
						Schema: "public", Name: "other",
						Columns: []*ir.Column{
							{Name: "id", DataType: "integer", Position: 1},
							{Name: "tags", DataType: "utils.hstore", ExtensionName: "hstore", Position: 2},
						},
					},
				},
				Types: map[string]*ir.Type{},
			},
		},
	}

	diffs := GenerateMigration(oldIR, newIR, "public")

	found := false
	for _, d := range diffs {
		for _, stmt := range d.Statements {
			if strings.Contains(stmt.SQL, "ADD COLUMN metadata") {
				found = true
				if !strings.Contains(stmt.SQL, "utils.hstore") {
					t.Errorf("expected ADD COLUMN to qualify with the real schema (utils), got: %q", stmt.SQL)
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected an ADD COLUMN metadata statement, got diffs: %+v", diffs)
	}
}

// This is the exact real-world regression: an extension installed via a
// setup/bootstrap script into a schema other than the target (e.g. hstore
// into "utils") but not yet referenced by ANY existing column - so there is
// no sibling column to infer the schema from, only oldIR.Extensions (sourced
// from pg_extension) knows where it really lives. Without that authoritative
// source, this case falls back to a bare, unqualified reference that fails
// at real apply time whenever the extension isn't in the target schema or
// public.
func TestGenerateMigration_AddColumnExtensionOwnedTypeKnownOnlyViaExtensionsField(t *testing.T) {
	oldIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {
						Schema: "public", Name: "users",
						Columns: []*ir.Column{{Name: "id", DataType: "integer", Position: 1}},
					},
				},
				Types: map[string]*ir.Type{},
			},
		},
		Extensions: map[string]string{"hstore": "utils"},
	}
	newIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name: "public",
				Tables: map[string]*ir.Table{
					"users": {
						Schema: "public", Name: "users",
						Columns: []*ir.Column{
							{Name: "id", DataType: "integer", Position: 1},
							{Name: "metadata", DataType: "public.hstore", ExtensionName: "hstore", Position: 2},
						},
					},
				},
				Types: map[string]*ir.Type{},
			},
		},
		Extensions: map[string]string{"hstore": "utils"},
	}

	diffs := GenerateMigration(oldIR, newIR, "public")

	found := false
	for _, d := range diffs {
		for _, stmt := range d.Statements {
			if strings.Contains(stmt.SQL, "ADD COLUMN metadata") {
				found = true
				if !strings.Contains(stmt.SQL, "utils.hstore") {
					t.Errorf("expected ADD COLUMN to qualify with the real schema (utils) from oldIR.Extensions, got: %q", stmt.SQL)
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected an ADD COLUMN metadata statement, got diffs: %+v", diffs)
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
