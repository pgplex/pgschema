package diff

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestPartitionIndexDependencyError(t *testing.T) {
	for _, tc := range []struct {
		name      string
		partition *ir.TableDependency
		message   string
	}{
		{
			name:    "excluded_index",
			message: `cannot dump index "App.Schema"."Parent.Index": attached index "App.Schema"."Child.Index" is excluded; adjust .pgschemaignore to include the child index, its table, and any owning constraint`,
		},
		{
			name:      "excluded_partition",
			partition: &ir.TableDependency{Schema: "App.Schema", Name: "Leaf.Table", Type: ir.DependencyTypeTable},
			message:   `cannot dump index "App.Schema"."Parent.Index": attached constraint-backed index "App.Schema"."Child.Index" requires excluded partition table "App.Schema"."Leaf.Table"; adjust .pgschemaignore to include the partition table`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := &PartitionIndexDependencyError{
				ParentIndex:   ir.IndexRef{Schema: "App.Schema", Name: "Parent.Index"},
				AttachedIndex: ir.IndexRef{Schema: "App.Schema", Name: "Child.Index"},
				ExcludedTable: tc.partition,
			}
			wrapped := fmt.Errorf("generate dump: %w", original)
			var restored *PartitionIndexDependencyError
			if !errors.As(wrapped, &restored) || restored != original {
				t.Fatalf("Lost dependency classification or context: %v", wrapped)
			}
			if got := restored.Error(); got != tc.message {
				t.Fatalf("Dependency guidance changed:\ngot: %s\nwant: %s", got, tc.message)
			}
		})
	}
}
