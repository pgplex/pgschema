package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestExplicitConstraintFamilyEligibility(t *testing.T) {
	for _, name := range []string{"complete", "split_batch", "existing_child", "filtered_child", "missing_definition", "missing_inherited", "ignored_origin", "incomplete_ancestry"} {
		t.Run(name, func(t *testing.T) {
			root := &ir.Table{Schema: "public", Name: "events", IsPartitioned: true, PartitionStrategy: "RANGE", PartitionKey: "id", Columns: []*ir.Column{{Name: "id", DataType: "integer"}}, Constraints: map[string]*ir.Constraint{"root_pk": {Schema: "public", Table: "events", Name: "root_pk", Type: ir.ConstraintTypePrimaryKey, Columns: []*ir.ConstraintColumn{{Name: "id", Position: 1}}, BackingIndex: &ir.IndexRef{Schema: "public", Name: "root_pk"}, CatalogDefinition: "PRIMARY KEY (id)"}}}
			child := &ir.Table{Schema: "public", Name: "events_leaf", PartitionOf: "events", PartitionBound: "FOR VALUES FROM (0) TO (10)", Columns: root.Columns, InheritedConstraintIndexes: []ir.InheritedConstraintIndex{{Index: ir.IndexRef{Schema: "public", Name: "leaf_pk"}, ConstraintName: "leaf_pk", ConstraintType: ir.ConstraintTypePrimaryKey, ParentIndex: ir.IndexRef{Schema: "public", Name: "root_pk"}, ParentConstraint: ir.ConstraintRef{Schema: "public", Table: "events", Name: "root_pk"}, Definition: "PRIMARY KEY (id)"}}}
			root.AllPartitionChildren = []ir.TableDependency{{Schema: "public", Name: child.Name}}
			tables := map[string]*ir.Table{"public.events": root, "public.events_leaf": child}
			added := []*ir.Table{root, child}
			batches := [][]*ir.Table{added}
			switch name {
			case "split_batch":
				batches = [][]*ir.Table{{root}, {child}}
			case "existing_child":
				added = []*ir.Table{root}
				batches = [][]*ir.Table{added}
			case "filtered_child":
				root.AllPartitionChildren = append(root.AllPartitionChildren, ir.TableDependency{Schema: "public", Name: "missing"})
			case "missing_definition":
				root.Constraints["root_pk"].CatalogDefinition = ""
			case "missing_inherited":
				child.InheritedConstraintIndexes = nil
				root.AllPartitionChildren = nil
			case "ignored_origin":
				root.Constraints = nil
			case "incomplete_ancestry":
				root.PartitionOf = "missing"
			}
			plan := planPartitionIndexCreation(added, tables, false)
			plan.planExplicitConstraints(tables, batches...)
			if got := len(plan.constraints) > 0; got != (name == "complete") {
				t.Fatalf("explicit selection=%v", got)
			}
			if name != "complete" {
				return
			}
			collector := newDiffCollector()
			generateCreateTablesSQL(added, "public", collector, nil, nil, nil, tables, plan)
			text := buildSQLFromSteps(collector.diffs)
			for _, required := range []string{"id integer NOT NULL", "ALTER TABLE ONLY events_leaf ADD CONSTRAINT leaf_pk PRIMARY KEY (id);", "ALTER TABLE ONLY events ADD CONSTRAINT root_pk PRIMARY KEY (id);", "ALTER INDEX root_pk ATTACH PARTITION leaf_pk;"} {
				if !strings.Contains(text, required) {
					t.Fatalf("missing %q in %s", required, text)
				}
			}
			if strings.Contains(text, "pgschema_restore") {
				t.Fatal("explicit family retained restoration")
			}
			if strings.Index(text, "ALTER TABLE ONLY events_leaf") > strings.Index(text, "ALTER TABLE ONLY events ADD") {
				t.Fatal("parent created before child")
			}
			fk := &ir.Constraint{Type: ir.ConstraintTypeForeignKey, ReferencedTable: "events"}
			for _, unrelated := range []*ir.Constraint{
				{Schema: "public", Table: "another_table", Name: "root_pk", Type: ir.ConstraintTypeCheck},
				{Schema: "public", Table: "events", Name: "root_pk", Type: ir.ConstraintTypeCheck},
				{Schema: "public", Table: "another_table", Name: "root_pk", Type: ir.ConstraintTypePrimaryKey},
			} {
				if plan.explicitConstraint(unrelated) {
					t.Fatalf("unrelated constraint selected: %+v", unrelated)
				}
			}
			if !plan.defersForeignKey(child, fk) {
				t.Fatal("PK foreign key was not deferred")
			}
		})
	}
}
