package diff

import (
	"fmt"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestTopologicallySortTablesHandlesCycles(t *testing.T) {
	tables := []*ir.Table{
		newTestTable("a"),
		newTestTable("b", "a"),
		newTestTable("c", "b"),
		newTestTable("x", "y"), // cycle x <-> y
		newTestTable("y", "x"),
		newTestTable("z", "y"), // depends on the cycle
	}

	sorted := topologicallySortTables(tables)
	if len(sorted) != len(tables) {
		t.Fatalf("expected %d tables, got %d", len(tables), len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, tbl := range sorted {
		order[tbl.Name] = idx
	}

	assertBefore := func(first, second string) {
		if order[first] >= order[second] {
			t.Fatalf("expected %s to appear before %s in %v", first, second, order)
		}
	}

	assertBefore("a", "b")
	assertBefore("b", "c")
	assertBefore("y", "z") // dependent tables still come afterwards

	// Cycle members should have a deterministic order (insertion order in this implementation)
	if order["x"] >= order["y"] {
		t.Fatalf("expected x to be ordered before y for deterministic output, got %v", order)
	}
}

// A column can reference (via nextval() default) a sequence it does NOT own,
// with no formal FK tying the two tables together (e.g. two tables sharing one
// sequence). topologicallySortTables must still order the referencing table
// after the table that genuinely owns the sequence, or CREATE TABLE can run
// before the sequence's owning table - and hence the sequence itself - exists.
func TestTopologicallySortTablesOrdersSharedSequenceReferenceAfterOwner(t *testing.T) {
	seqDefault := `nextval('"owner_seq"'::regclass)`

	owner := &ir.Table{
		Schema: "public", Name: "owner",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "id", DefaultValue: &seqDefault, HasOwnedSequence: true},
		},
	}
	sharer := &ir.Table{
		Schema: "public", Name: "sharer",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "owner_id", DefaultValue: &seqDefault, HasOwnedSequence: false},
		},
	}
	unrelated := newTestTable("unrelated")

	// Feed sharer before owner to prove the ordering isn't just insertion order.
	sorted := topologicallySortTables([]*ir.Table{sharer, unrelated, owner})
	if len(sorted) != 3 {
		t.Fatalf("expected 3 tables, got %d", len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, tbl := range sorted {
		order[tbl.Name] = idx
	}

	if order["owner"] >= order["sharer"] {
		t.Fatalf("expected owner to be ordered before sharer, got %v", order)
	}
}

// Two different schemas can each genuinely own a sequence with the same bare
// name. A referencer in one schema must be ordered after its own schema's
// owner, not the other schema's same-named owner - proving sequenceOwnerTable
// is keyed by schema-qualified name, not bare name.
func TestTopologicallySortTablesOrdersSharedSequenceReferenceBySchemaNotBareName(t *testing.T) {
	domainDefault := `nextval('domain.shared_seq'::regclass)`
	publicDefault := `nextval('shared_seq'::regclass)`

	domainOwner := &ir.Table{
		Schema: "domain", Name: "owner",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "id", DefaultValue: &domainDefault, HasOwnedSequence: true},
		},
	}
	domainSharer := &ir.Table{
		Schema: "domain", Name: "sharer",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "owner_id", DefaultValue: &domainDefault, HasOwnedSequence: false},
		},
	}
	publicOwner := &ir.Table{
		Schema: "public", Name: "owner",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "id", DefaultValue: &publicDefault, HasOwnedSequence: true},
		},
	}
	publicSharer := &ir.Table{
		Schema: "public", Name: "sharer",
		Constraints: map[string]*ir.Constraint{},
		Columns: []*ir.Column{
			{Name: "owner_id", DefaultValue: &publicDefault, HasOwnedSequence: false},
		},
	}

	sorted := topologicallySortTables([]*ir.Table{domainSharer, publicSharer, publicOwner, domainOwner})
	if len(sorted) != 4 {
		t.Fatalf("expected 4 tables, got %d", len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, tbl := range sorted {
		order[tbl.Schema+"."+tbl.Name] = idx
	}

	if order["domain.owner"] >= order["domain.sharer"] {
		t.Fatalf("expected domain.owner before domain.sharer, got %v", order)
	}
	if order["public.owner"] >= order["public.sharer"] {
		t.Fatalf("expected public.owner before public.sharer, got %v", order)
	}
}

func TestNextvalTargetSequenceKey(t *testing.T) {
	col := func(defaultValue string) *ir.Column {
		return &ir.Column{DefaultValue: &defaultValue}
	}

	tests := []struct {
		name           string
		column         *ir.Column
		fallbackSchema string
		want           string
	}{
		{"unqualified bare sequence name -> uses fallback schema",
			col(`nextval('owner_seq'::regclass)`), "public", "public.owner_seq"},
		{"unqualified quoted sequence name -> uses fallback schema",
			col(`nextval('"owner_seq"'::regclass)`), "public", "public.owner_seq"},
		{"schema-qualified sequence name -> uses its own schema, not the fallback",
			col(`nextval('domain.owner_seq'::regclass)`), "public", "domain.owner_seq"},
		{"schema-qualified, both parts quoted",
			col(`nextval('"Domain"."Owner_Seq"'::regclass)`), "public", "Domain.Owner_Seq"},
		{"quoted identifier containing a literal dot, no schema qualifier",
			// The whole reference is one quoted identifier ("owner.seq"), not a
			// schema-qualified name - a naive dot-split would wrongly treat
			// "owner" as the schema and "seq" as the name.
			col(`nextval('"owner.seq"'::regclass)`), "public", "public.owner.seq"},
		{"quoted identifier containing an escaped single quote",
			// PostgreSQL escapes a literal single quote inside a quoted
			// identifier, when placed in a string literal, by doubling it.
			// A regex that stops at the first single quote would truncate
			// this to just `"O`, losing `'Reilly_seq"` and everything after.
			col(`nextval('"O''Reilly_seq"'::regclass)`), "public", "public.O'Reilly_seq"},
		{"no default value -> empty",
			&ir.Column{}, "public", ""},
		{"default value without nextval -> empty",
			col("0"), "public", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextvalTargetSequenceKey(tt.column, tt.fallbackSchema); got != tt.want {
				t.Errorf("nextvalTargetSequenceKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func newTestTable(name string, deps ...string) *ir.Table {
	constraints := make(map[string]*ir.Constraint)
	for idx, dep := range deps {
		constraints[fmt.Sprintf("fk_%s_%d", name, idx)] = &ir.Constraint{
			Type:             ir.ConstraintTypeForeignKey,
			ReferencedSchema: "public",
			ReferencedTable:  dep,
		}
	}

	return &ir.Table{
		Schema:      "public",
		Name:        name,
		Constraints: constraints,
	}
}

func TestTopologicallySortTypesHandlesCycles(t *testing.T) {
	types := []*ir.Type{
		// Simple chain: a <- b <- c
		newTestEnumType("a"),
		newTestCompositeType("b", "a"),
		newTestCompositeType("c", "b"),
		// Cycle: x <-> y (theoretically impossible in PostgreSQL but test handles it)
		newTestCompositeType("x", "y"),
		newTestCompositeType("y", "x"),
		// Type depending on the cycle
		newTestCompositeType("z", "y"),
	}

	sorted := topologicallySortTypes(types)
	if len(sorted) != len(types) {
		t.Fatalf("expected %d types, got %d", len(types), len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}

	assertBefore := func(first, second string) {
		if order[first] >= order[second] {
			t.Fatalf("expected %s to appear before %s in %v", first, second, order)
		}
	}

	// Verify simple chain ordering
	assertBefore("a", "b")
	assertBefore("b", "c")
	// Dependent types should still come after cycle members
	assertBefore("y", "z")

	// Cycle members should have a deterministic order (insertion order)
	if order["x"] >= order["y"] {
		t.Fatalf("expected x to be ordered before y for deterministic output, got %v", order)
	}
}

func TestTopologicallySortTypesMultipleNoDependencies(t *testing.T) {
	types := []*ir.Type{
		newTestEnumType("z"),
		newTestEnumType("a"),
		newTestEnumType("m"),
		newTestEnumType("b"),
	}

	sorted := topologicallySortTypes(types)
	if len(sorted) != len(types) {
		t.Fatalf("expected %d types, got %d", len(types), len(sorted))
	}

	// With no dependencies, should maintain deterministic alphabetical order
	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}

	// Verify deterministic ordering: a < b < m < z
	if order["a"] >= order["b"] || order["b"] >= order["m"] || order["m"] >= order["z"] {
		t.Fatalf("expected alphabetical order for types with no dependencies, got %v", order)
	}
}

func TestTopologicallySortTypesDomainReferencingCustomType(t *testing.T) {
	types := []*ir.Type{
		newTestEnumType("status_type"),
		newTestDomainType("status_domain", "status_type"),
		newTestCompositeType("person", "status_domain"),
	}

	sorted := topologicallySortTypes(types)
	if len(sorted) != len(types) {
		t.Fatalf("expected %d types, got %d", len(types), len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}

	assertBefore := func(first, second string) {
		if order[first] >= order[second] {
			t.Fatalf("expected %s to appear before %s in %v", first, second, order)
		}
	}

	// Verify correct dependency chain
	assertBefore("status_type", "status_domain")
	assertBefore("status_domain", "person")
}

func TestTopologicallySortTypesCompositeWithMultipleDependencies(t *testing.T) {
	types := []*ir.Type{
		newTestEnumType("status"),
		newTestEnumType("priority"),
		newTestEnumType("category"),
		newTestCompositeType("task", "status", "priority", "category"),
		newTestCompositeType("project", "task"),
	}

	sorted := topologicallySortTypes(types)
	if len(sorted) != len(types) {
		t.Fatalf("expected %d types, got %d", len(types), len(sorted))
	}

	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}

	assertBefore := func(first, second string) {
		if order[first] >= order[second] {
			t.Fatalf("expected %s to appear before %s in %v", first, second, order)
		}
	}

	// All dependencies should come before task
	assertBefore("status", "task")
	assertBefore("priority", "task")
	assertBefore("category", "task")
	// And task should come before project
	assertBefore("task", "project")
}

func TestExtractTypeNameNormalizesQuoting(t *testing.T) {
	// Post-#493 the inspector emits user-defined type references via
	// quote_ident (schema-qualified). extractTypeName must normalize them to the
	// same delimiter-safe key that typeGraphKey builds for typeMap entries.
	cases := []struct{ in, schema, wantSchema, wantName string }{
		{"user_kind", "public", "public", "user_kind"},           // bare -> default schema
		{"public.user_kind", "public", "public", "user_kind"},    // already qualified
		{`public."user"`, "public", "public", "user"},            // quoted reserved-word name
		{`"MySchema"."MyType"`, "public", "MySchema", "MyType"},   // quoted schema + name
		{"public.user_kind[]", "public", "public", "user_kind"},  // array notation stripped
		{`other."odd.name"`, "public", "other", "odd.name"},      // dot inside quotes is not a separator
		{`"a.b".c`, "public", "a.b", "c"},                        // dotted schema, bare type
		{"public.vector(384)", "public", "public", "vector"},     // typmod suffix stripped
		{"s.num(10, 2)", "public", "s", "num"},                   // typmod with comma/space stripped
		{`a."b"".c"`, "public", "a", `b".c`},                     // escaped quote ("") + dot inside a quoted ident
	}
	for _, c := range cases {
		want := typeGraphKey(c.wantSchema, c.wantName)
		if got := extractTypeName(c.in, c.schema); got != want {
			t.Errorf("extractTypeName(%q, %q) = %q, want %q", c.in, c.schema, got, want)
		}
	}

	// The delimiter-safe key keeps legal-but-dotted identifiers distinct:
	// schema "a.b" type "c" must not collide with schema "a" type "b.c".
	if k1, k2 := extractTypeName(`"a.b".c`, "public"), extractTypeName(`a."b.c"`, "public"); k1 == k2 {
		t.Errorf("dotted identifiers collided: both keyed as %q", k1)
	}
}

func TestTopologicallySortTypesQualifiedQuotedRefs(t *testing.T) {
	// The inspector now qualifies same-schema composite-attribute and domain
	// base-type references (e.g. public."odd name"). Dependency edges must
	// still be found against bare typeMap keys.
	referenced := &ir.Type{
		Schema:     "public",
		Name:       "odd name", // needs quote_ident
		Kind:       ir.TypeKindEnum,
		EnumValues: []string{"a", "b"},
	}
	composite := &ir.Type{
		Schema: "public",
		Name:   "holder",
		Kind:   ir.TypeKindComposite,
		Columns: []*ir.TypeColumn{
			{Name: "c", DataType: `public."odd name"`, Position: 1},
		},
	}
	dom := &ir.Type{
		Schema:   "public",
		Name:     "odd domain",
		Kind:     ir.TypeKindDomain,
		BaseType: `public."odd name"`,
	}

	sorted := topologicallySortTypes([]*ir.Type{composite, referenced, dom})
	if len(sorted) != 3 {
		t.Fatalf("expected 3 types, got %d", len(sorted))
	}
	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}
	if order["odd name"] >= order["holder"] {
		t.Fatalf(`expected "odd name" before holder in %v`, order)
	}
	if order["odd name"] >= order["odd domain"] {
		t.Fatalf(`expected "odd name" before "odd domain" in %v`, order)
	}
}

func newTestEnumType(name string) *ir.Type {
	return &ir.Type{
		Schema:     "public",
		Name:       name,
		Kind:       ir.TypeKindEnum,
		EnumValues: []string{"value1", "value2"},
	}
}

func newTestCompositeType(name string, deps ...string) *ir.Type {
	columns := make([]*ir.TypeColumn, len(deps))
	for idx, dep := range deps {
		columns[idx] = &ir.TypeColumn{
			Name:     fmt.Sprintf("col_%d", idx),
			DataType: dep, // References the type
			Position: idx + 1,
		}
	}

	return &ir.Type{
		Schema:  "public",
		Name:    name,
		Kind:    ir.TypeKindComposite,
		Columns: columns,
	}
}

func newTestDomainType(name, baseType string) *ir.Type {
	return &ir.Type{
		Schema:   "public",
		Name:     name,
		Kind:     ir.TypeKindDomain,
		BaseType: baseType,
	}
}

func TestBuildFunctionBodyDependencies(t *testing.T) {
	tests := []struct {
		name     string
		funcs    []*ir.Function
		expected map[string][]string // function name -> expected dependency names
	}{
		{
			name: "simple dependency: wrapper calls helper",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "wrapper",
					Definition: "SELECT helper()",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "helper",
					Definition: "SELECT 1",
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"wrapper": {"public.helper()"},
				"helper":  nil,
			},
		},
		{
			name: "qualified function call",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "caller",
					Definition: "SELECT public.callee()",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "callee",
					Definition: "SELECT 1",
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"caller": {"public.callee()"},
				"callee": nil,
			},
		},
		{
			name: "chain: a calls b calls c",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "func_a",
					Definition: "SELECT func_b()",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "func_b",
					Definition: "SELECT func_c()",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "func_c",
					Definition: "SELECT 1",
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"func_a": {"public.func_b()"},
				"func_b": {"public.func_c()"},
				"func_c": nil,
			},
		},
		{
			name: "no self-dependency",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "recursive",
					Definition: "SELECT recursive()", // calls itself
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"recursive": nil, // should not add self as dependency
			},
		},
		{
			name: "multiple calls in body",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "orchestrator",
					Definition: "SELECT step_one() + step_two() + step_three()",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "step_one",
					Definition: "SELECT 1",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "step_two",
					Definition: "SELECT 2",
					Language:   "sql",
				},
				{
					Schema:     "public",
					Name:       "step_three",
					Definition: "SELECT 3",
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"orchestrator": {"public.step_one()", "public.step_two()", "public.step_three()"},
				"step_one":     nil,
				"step_two":     nil,
				"step_three":   nil,
			},
		},
		{
			name: "external function not tracked",
			funcs: []*ir.Function{
				{
					Schema:     "public",
					Name:       "my_func",
					Definition: "SELECT pg_catalog.now() + external_func()",
					Language:   "sql",
				},
			},
			expected: map[string][]string{
				"my_func": nil, // external functions not in our list
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any existing dependencies
			for _, fn := range tt.funcs {
				fn.Dependencies = nil
			}

			buildFunctionBodyDependencies(tt.funcs)

			for _, fn := range tt.funcs {
				expectedDeps := tt.expected[fn.Name]

				if len(fn.Dependencies) != len(expectedDeps) {
					t.Errorf("function %s: expected %d dependencies, got %d: %v",
						fn.Name, len(expectedDeps), len(fn.Dependencies), fn.Dependencies)
					continue
				}

				// Check each expected dependency exists
				for _, exp := range expectedDeps {
					found := false
					for _, dep := range fn.Dependencies {
						if dep == exp {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("function %s: expected dependency %s not found in %v",
							fn.Name, exp, fn.Dependencies)
					}
				}
			}
		})
	}
}

func TestBuildFunctionBodyDependenciesWithTopologicalSort(t *testing.T) {
	// Integration test: build dependencies then sort
	functions := []*ir.Function{
		{
			Schema:     "public",
			Name:       "z_wrapper", // alphabetically last, but should come last after sort
			Definition: "SELECT a_helper()",
			Language:   "sql",
		},
		{
			Schema:     "public",
			Name:       "a_helper", // alphabetically first
			Definition: "SELECT 1",
			Language:   "sql",
		},
	}

	// Build dependencies from function bodies
	buildFunctionBodyDependencies(functions)

	// Verify dependency was detected
	if len(functions[0].Dependencies) != 1 {
		t.Fatalf("expected z_wrapper to have 1 dependency, got %d", len(functions[0].Dependencies))
	}

	// Now sort
	sorted := topologicallySortFunctions(functions)

	// a_helper should come before z_wrapper
	order := make(map[string]int)
	for i, fn := range sorted {
		order[fn.Name] = i
	}

	if order["a_helper"] >= order["z_wrapper"] {
		t.Errorf("expected a_helper before z_wrapper, got order: %v", order)
	}
}
