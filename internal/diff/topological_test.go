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

func TestSortDeferredForeignKeys_ordersRefBeforeChild(t *testing.T) {
	child := &ir.Table{Schema: "app", Name: "child"}
	fk := &ir.Constraint{
		Name:              "child_parent_fk",
		Type:              ir.ConstraintTypeForeignKey,
		Columns:           []*ir.ConstraintColumn{{Name: "pid", Position: 1}},
		ReferencedSchema:  "public",
		ReferencedTable:   "parent",
		ReferencedColumns: []*ir.ConstraintColumn{{Name: "id", Position: 1}},
	}
	fk2 := &ir.Constraint{
		Name:              "other_child_fk",
		Type:              ir.ConstraintTypeForeignKey,
		Columns:           []*ir.ConstraintColumn{{Name: "cid", Position: 1}},
		ReferencedSchema:  "app",
		ReferencedTable:   "child",
		ReferencedColumns: []*ir.ConstraintColumn{{Name: "id", Position: 1}},
	}
	other := &ir.Table{Schema: "public", Name: "other"}
	items2 := []*deferredConstraint{
		{table: other, constraint: fk2},
		{table: child, constraint: fk},
	}
	sorted2 := sortDeferredForeignKeys(items2)
	if len(sorted2) != 2 {
		t.Fatalf("got %d", len(sorted2))
	}
	if sorted2[0].table.Name != "child" || sorted2[1].table.Name != "other" {
		t.Fatalf("expected child FK before other FK, got %v then %v", sorted2[0].table.Name, sorted2[1].table.Name)
	}
}
