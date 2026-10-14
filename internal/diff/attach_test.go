package diff

import (
	"encoding/json"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestDiffOperationSerialization(t *testing.T) {
	for value, name := range []string{"create", "alter", "drop", "recreate", "attach"} {
		op := DiffOperation(value)
		if op.String() != name {
			t.Fatalf("operation %d = %q, want %q", value, op.String(), name)
		}
		encoded, err := json.Marshal(op)
		if err != nil || string(encoded) != `"`+name+`"` {
			t.Fatalf("marshal %s = %s, %v", name, encoded, err)
		}
		var restored DiffOperation
		if err := json.Unmarshal(encoded, &restored); err != nil || restored != op {
			t.Fatalf("unmarshal %s = %v, %v", name, restored, err)
		}
	}
	for _, input := range []string{`"unknown"`, `4`, `true`} {
		var op DiffOperation
		if err := json.Unmarshal([]byte(input), &op); err == nil {
			t.Errorf("accepted invalid operation %s", input)
		}
	}
}

func TestPartitionIndexAttachmentMetadata(t *testing.T) {
	schema := partitionIndexTestSchema()
	parent := schema.Schemas["public"].Tables["events"].Indexes["parent_lookup"]
	dumps, err := GenerateDump(schema, "public", false)
	if err != nil {
		t.Fatal(err)
	}
	for name, diffs := range map[string][]Diff{
		"dump":      dumps,
		"migration": GenerateMigration(ir.NewIR(), schema, "public"),
	} {
		t.Run(name, func(t *testing.T) {
			attachments := 0
			for _, d := range diffs {
				if d.Operation != DiffOperationAttach {
					continue
				}
				attachments++
				if d.Type != DiffTypeTableIndex || d.Path != "public.events.parent_lookup" || d.Source != parent {
					t.Fatalf("attachment ownership changed: %+v", d)
				}
				if len(d.Statements) != 1 || d.Statements[0].SQL != "ALTER INDEX parent_lookup ATTACH PARTITION custom_lookup;" || !d.Statements[0].CanRunInTransaction {
					t.Fatalf("attachment SQL changed: %+v", d.Statements)
				}
			}
			if attachments != 1 {
				t.Fatalf("got %d attachments, want 1", attachments)
			}
		})
	}
}
