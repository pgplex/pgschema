package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestInheritedConstraintIndexEligibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"complete", true}, {"ignored_origin", false}, {"existing_root", false}, {"existing_child", false},
		{"filtered_child", false}, {"unmanaged_ancestor", false}, {"nested_retained_origin", true}, {"nested_ignored_origin", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := partitionIndexTestSchema()
			tables := schema.Schemas["public"].Tables
			root, child := tables["events"], tables["events_early"]
			origin := &ir.Constraint{Schema: "public", Table: root.Name, Name: "origin", Type: ir.ConstraintTypePrimaryKey}
			root.Constraints = map[string]*ir.Constraint{"origin": origin}
			root.AllPartitionChildren = []ir.TableDependency{{Schema: "public", Name: child.Name}}
			child.InheritedConstraintIndexes = []ir.InheritedConstraintIndex{{Index: ir.IndexRef{Schema: "public", Name: "renamed_leaf"}, ConstraintName: "renamed_leaf", ConstraintType: ir.ConstraintTypePrimaryKey, ParentIndex: ir.IndexRef{Schema: "public", Name: "origin"}, ParentConstraint: ir.ConstraintRef{Schema: "public", Table: root.Name, Name: "origin"}}}
			desired := map[string]*ir.Table{"public.events": root, "public.events_early": child}
			added := []*ir.Table{root, child}
			switch tc.name {
			case "ignored_origin":
				delete(root.Constraints, "origin")
			case "existing_root":
				added = []*ir.Table{child}
			case "existing_child":
				added = []*ir.Table{root}
			case "filtered_child":
				root.AllPartitionChildren = append(root.AllPartitionChildren, ir.TableDependency{Schema: "public", Name: "missing"})
			case "unmanaged_ancestor":
				root.PartitionOf = "external"
				root.PartitionOfSchema = "other"
			case "nested_retained_origin", "nested_ignored_origin":
				ancestor := &ir.Table{Schema: "public", Name: "ancestor", Constraints: map[string]*ir.Constraint{"ancestor_origin": {Name: "ancestor_origin", Type: ir.ConstraintTypePrimaryKey}}}
				ancestor.AllPartitionChildren = []ir.TableDependency{{Schema: "public", Name: root.Name}}
				root.Constraints = nil
				root.PartitionOf = ancestor.Name
				root.PartitionOfSchema = "public"
				root.InheritedConstraintIndexes = []ir.InheritedConstraintIndex{{Index: ir.IndexRef{Schema: "public", Name: "origin"}, ConstraintName: "origin", ConstraintType: ir.ConstraintTypePrimaryKey, ParentIndex: ir.IndexRef{Schema: "public", Name: "ancestor_origin"}, ParentConstraint: ir.ConstraintRef{Schema: "public", Table: ancestor.Name, Name: "ancestor_origin"}}}
				desired["public.ancestor"] = ancestor
				added = append(added, ancestor)
				tables[ancestor.Name] = ancestor
				if tc.name == "nested_ignored_origin" {
					ancestor.Constraints = nil
				}
			}
			plan := planPartitionIndexCreation(added, desired, false)
			restoration := plan.restoreInheritedIndexesSQL(child, "public", false)
			if (restoration != "") != tc.want {
				t.Fatalf("restoration eligibility = %v, want %v", restoration != "", tc.want)
			}
			if tc.want {
				statements := GenerateMigration(ir.NewIR(), schema, "public")
				if !strings.Contains(buildSQLFromSteps(statements), "INTO STRICT") {
					t.Fatal("migration omitted restoration")
				}
				found := false
				for _, step := range statements {
					if step.Type == DiffTypeTable && step.Path == "public.events_early" {
						if len(step.Statements) != 2 || !strings.HasPrefix(step.Statements[0].SQL, "CREATE TABLE") || !strings.HasPrefix(step.Statements[1].SQL, "DO ") || !step.Statements[1].CanRunInTransaction {
							t.Fatalf("restoration is not part of table creation: %+v", step)
						}
						found = true
					}
				}
				if !found {
					t.Fatal("missing table creation")
				}
			}
		})
	}
}
