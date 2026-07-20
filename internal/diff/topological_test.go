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
