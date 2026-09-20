package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/stretchr/testify/require"
)

func TestIndexRecoveryClassification(t *testing.T) {
	for _, tc := range []struct {
		name      string
		mutate    func(*ir.Schema, *ir.Schema)
		wantError string
		repairs   int
	}{
		{name: "unready", repairs: 1},
		{name: "ready_unique", repairs: 1, mutate: func(a, b *ir.Schema) {
			a.IndexStates["idx"].Ready = true
			a.Tables["t"].Indexes["idx"].Type = ir.IndexTypeUnique
			b.Tables["t"].Indexes["idx"].Type = ir.IndexTypeUnique
		}},
		{name: "comment_only", repairs: 1, mutate: func(a, b *ir.Schema) { b.Tables["t"].Indexes["idx"].Comment = "new comment" }},
		{name: "healthy", mutate: func(a, b *ir.Schema) { a.IndexStates = nil }},
		{name: "active", wantError: "active", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].BuildInProgress = true }},
		{name: "maintenance", wantError: "maintenance", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].ConflictingOperation = true }},
		{name: "dead_required", wantError: "unsupported catalog state", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].Live = false }},
		{name: "constraint", wantError: "constraint-backed", mutate: func(a, b *ir.Schema) {
			a.IndexStates["idx"].Constraint = "unique_constraint"
			b.Tables["t"].Constraints = map[string]*ir.Constraint{"unique_constraint": {Name: "unique_constraint"}}
		}},
		{name: "undesired_constraint", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].Constraint = "removed_constraint" }},
		{name: "undesired_partition_constraint", mutate: func(a, b *ir.Schema) {
			a.IndexStates["idx"].Constraint = "removed_constraint"
			a.IndexStates["idx"].IndexKind = "I"
		}},
		{name: "desired_unhealthy", wantError: "planning database", mutate: func(a, b *ir.Schema) {
			v := *a.IndexStates["idx"]
			b.IndexStates = map[string]*ir.IndexState{"idx": &v}
		}},
		{name: "changed_unique_definition", wantError: "requested definition change", mutate: func(a, b *ir.Schema) { a.Tables["t"].Indexes["idx"].Type = ir.IndexTypeUnique }},
		{name: "undesired_invalid", mutate: func(a, b *ir.Schema) { delete(b.Tables["t"].Indexes, "idx") }},
		{name: "undesired_dead_cleanup", mutate: func(a, b *ir.Schema) { delete(b.Tables["t"].Indexes, "idx"); a.IndexStates["idx"].Live = false }},
		{name: "partition_index_removed", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].IndexKind = "I"; delete(b.Tables["t"].Indexes, "idx") }},
		{name: "partition_table_removed", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].IndexKind = "I"; delete(b.Tables, "t") }},
		{name: "partition_incomplete", wantError: "attach", mutate: func(a, b *ir.Schema) { a.IndexStates["idx"].IndexKind = "I" }},
		{name: "intentional_partition_with_maintenance", mutate: func(a, b *ir.Schema) {
			a.IndexStates["idx"].IndexKind = "I"
			a.IndexStates["idx"].ConflictingOperation = true
			v := *a.IndexStates["idx"]
			b.IndexStates = map[string]*ir.IndexState{"idx": &v}
		}},
		{name: "intentional_partition", mutate: func(a, b *ir.Schema) {
			a.IndexStates["idx"].IndexKind = "I"
			v := *a.IndexStates["idx"]
			b.IndexStates = map[string]*ir.IndexState{"idx": &v}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makeSchema := func() *ir.Schema {
				return &ir.Schema{Name: "public", Tables: map[string]*ir.Table{"t": {Indexes: map[string]*ir.Index{"idx": {Name: "idx", Schema: "public", Table: "t", Type: ir.IndexTypeRegular, Method: "btree", Columns: []*ir.IndexColumn{{Name: "id", Position: 1}}}}}}}
			}
			a, b := makeSchema(), makeSchema()
			a.IndexStates = map[string]*ir.IndexState{"idx": {Table: "t", IndexKind: "i", TableKind: "r", Live: true}}
			if tc.mutate != nil {
				tc.mutate(a, b)
			}
			repairs, err := IndexRecoveryDiffs(&ir.IR{Schemas: map[string]*ir.Schema{"public": a}}, &ir.IR{Schemas: map[string]*ir.Schema{"public": b}}, "public")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				require.Empty(t, repairs)
			} else {
				require.NoError(t, err)
				require.Len(t, repairs, tc.repairs)
			}
		})
	}
}
