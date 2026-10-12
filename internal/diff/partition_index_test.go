package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func partitionIndexTestSchema() *ir.IR {
	parent := &ir.Index{
		Schema: "public", Table: "events", Name: "parent_lookup",
		Type: ir.IndexTypeRegular, Method: "btree", IsPartitioned: true,
		Columns:            []*ir.IndexColumn{{Name: "id", Position: 1}},
		AttachedPartitions: []ir.IndexRef{{Schema: "public", Name: "custom_lookup"}},
	}
	child := &ir.Index{
		Schema: "public", Table: "events_early", Name: "custom_lookup",
		Type: ir.IndexTypeRegular, Method: "btree",
		Columns:     []*ir.IndexColumn{{Name: "id", Position: 1}},
		ParentIndex: parent.Name, ParentIndexSchema: parent.Schema,
	}
	schema := ir.NewIR()
	schema.Schemas["public"] = &ir.Schema{Name: "public", Tables: map[string]*ir.Table{
		"events": {
			Schema: "public", Name: "events", IsPartitioned: true,
			PartitionStrategy: "RANGE", PartitionKey: "id",
			Columns: []*ir.Column{{Name: "id", Position: 1, DataType: "integer"}},
			Indexes: map[string]*ir.Index{parent.Name: parent},
		},
		"events_early": {
			Schema: "public", Name: "events_early", PartitionOf: "events", PartitionOfSchema: "public",
			PartitionBound: "FOR VALUES FROM (0) TO (10)",
			Columns:        []*ir.Column{{Name: "id", Position: 1, DataType: "integer"}},
			Indexes:        map[string]*ir.Index{child.Name: child},
		},
	}}
	return schema
}

// TestGenerateDumpPartitionIndexOrder checks the shared creation order for a
// complete new hierarchy independently of the dump formatter.
func TestGenerateDumpPartitionIndexOrder(t *testing.T) {
	schema := partitionIndexTestSchema()
	wantOrder := []string{
		"CREATE TABLE IF NOT EXISTS events (",
		"CREATE TABLE IF NOT EXISTS events_early PARTITION OF events",
		"CREATE INDEX IF NOT EXISTS custom_lookup ON events_early (id);",
		"CREATE INDEX IF NOT EXISTS parent_lookup ON ONLY events (id);",
		"ALTER INDEX parent_lookup ATTACH PARTITION custom_lookup;",
	}
	var first string
	for run := 0; run < 5; run++ {
		steps, err := GenerateDump(schema, "public", false)
		if err != nil {
			t.Fatal(err)
		}
		sql := buildSQLFromSteps(steps)
		previous := -1
		for _, stmt := range wantOrder {
			position := strings.Index(sql, stmt)
			if position <= previous {
				t.Fatalf("Missing or misplaced statement %q:\n%s", stmt, sql)
			}
			previous = position
		}
		if run == 0 {
			first = sql
		} else if sql != first {
			t.Fatal("Non-deterministic partitioned index export")
		}
	}
	migration := buildSQLFromSteps(GenerateMigration(ir.NewIR(), schema, "public"))
	if migration != first {
		t.Fatalf("New hierarchy migration differs from dump:\n%s", migration)
	}
}

func TestGenerateDumpExcludedPartitionIndex(t *testing.T) {
	for _, childSchema := range []string{"public", "other"} {
		t.Run(childSchema, func(t *testing.T) {
			parent := &ir.Index{
				Schema: "public", Table: "events", Name: "parent_lookup",
				Type: ir.IndexTypeRegular, Method: "btree", IsPartitioned: true,
				Columns:            []*ir.IndexColumn{{Name: "id", Position: 1}},
				AttachedPartitions: []ir.IndexRef{{Schema: "public", Name: "child_lookup"}},
			}
			child := &ir.Index{Schema: childSchema, Table: "events_early", Name: "child_lookup"}
			schema := ir.NewIR()
			schema.Schemas["public"] = &ir.Schema{Name: "public", Tables: map[string]*ir.Table{
				"events": {
					Schema: "public", Name: "events", IsPartitioned: true,
					PartitionStrategy: "RANGE", PartitionKey: "id",
					Columns: []*ir.Column{{Name: "id", Position: 1, DataType: "integer"}},
					Indexes: map[string]*ir.Index{parent.Name: parent},
				},
			}}
			if childSchema != "public" {
				// A same-named index in another schema cannot satisfy the attachment.
				schema.Schemas[childSchema] = &ir.Schema{Tables: map[string]*ir.Table{
					"events_early": {Indexes: map[string]*ir.Index{child.Name: child}},
				}}
			}
			steps, err := GenerateDump(schema, "public", false)
			if err == nil || !strings.Contains(err.Error(), "index public.parent_lookup: attached index public.child_lookup is excluded") {
				t.Fatalf("Expected excluded-child error, got %v", err)
			}
			if len(steps) != 0 {
				t.Fatal("Failed dump returned partial SQL")
			}
			if childSchema == "public" {
				migration := buildSQLFromSteps(GenerateMigration(ir.NewIR(), schema, "public"))
				if !strings.Contains(migration, "CREATE INDEX IF NOT EXISTS parent_lookup ON events (id);") {
					t.Fatalf("Dump validation changed migration generation:\n%s", migration)
				}
			}
		})
	}
}

func TestPartitionIndexCreationEligibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{name: "complete", want: true},
		{name: "unrelated_existing_hierarchy", want: true},
		{name: "intentionally_unattached", want: true},
		{name: "ignored_child"},
		{name: "ignored_parent"},
		{name: "ignored_table_without_attachments"},
		{name: "external_child"},
		{name: "existing_root"},
		{name: "existing_child"},
		{name: "unmanaged_ancestor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema := partitionIndexTestSchema()
			root := schema.Schemas["public"].Tables["events"]
			child := schema.Schemas["public"].Tables["events_early"]
			parentIndex := root.Indexes["parent_lookup"]
			childIndex := child.Indexes["custom_lookup"]
			parentIndex.Type = ir.IndexTypeUnique
			root.AllPartitionChildren = []ir.TableDependency{{Schema: "public", Name: "events_early"}}
			desired := map[string]*ir.Table{"public.events": root, "public.events_early": child}
			added := []*ir.Table{root, child}
			switch tc.name {
			case "unrelated_existing_hierarchy":
				desired["public.old_events"] = &ir.Table{Schema: "public", Name: "old_events", IsPartitioned: true}
			case "intentionally_unattached":
				parentIndex.AttachedPartitions = nil
				childIndex.ParentIndex = ""
				childIndex.ParentIndexSchema = ""
			case "ignored_child":
				delete(child.Indexes, childIndex.Name)
			case "ignored_parent":
				delete(root.Indexes, parentIndex.Name)
			case "ignored_table_without_attachments":
				parentIndex.AttachedPartitions = nil
				delete(desired, "public.events_early")
				added = []*ir.Table{root}
			case "external_child":
				root.AllPartitionChildren = append(root.AllPartitionChildren, ir.TableDependency{Schema: "other", Name: "external_events"})
			case "existing_root":
				added = []*ir.Table{child}
			case "existing_child":
				added = []*ir.Table{root}
			case "unmanaged_ancestor":
				root.PartitionOf = "outside"
				root.PartitionOfSchema = "other"
			}
			plan := planPartitionIndexCreation(added, desired, false)
			if got := plan.contains(parentIndex); got != tc.want {
				t.Fatalf("Expected scheduled parent = %v, got %v", tc.want, got)
			}
			fk := &ir.Constraint{Type: ir.ConstraintTypeForeignKey, ReferencedTable: "events"}
			if got := plan.defersForeignKey(&ir.Table{Schema: "public"}, fk); got != tc.want {
				t.Fatalf("FK deferral must match scheduled unique indexes: got %v, want %v", got, tc.want)
			}
			collector := newDiffCollector()
			plan.generateSQL("public", collector)
			if !tc.want && len(collector.diffs) != 0 {
				t.Fatalf("Excluded hierarchy emitted scheduled SQL: %+v", collector.diffs)
			}
			if tc.name == "intentionally_unattached" && strings.Contains(buildSQLFromSteps(collector.diffs), "ATTACH PARTITION") {
				t.Fatal("Unattached child was attached implicitly")
			}
		})
	}
}
