package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// TestGenerateDumpPartitionIndexOrder keeps the export-only index strategy out
// of migration generation and checks ordering independently of the formatter.
func TestGenerateDumpPartitionIndexOrder(t *testing.T) {
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
	if strings.Contains(migration, "ON ONLY") || strings.Contains(migration, "ATTACH PARTITION") {
		t.Fatalf("Dump-only index strategy changed migration generation:\n%s", migration)
	}
	if !strings.Contains(migration, "CREATE INDEX IF NOT EXISTS parent_lookup ON events (id);") {
		t.Fatalf("Expected existing recursive migration strategy:\n%s", migration)
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
