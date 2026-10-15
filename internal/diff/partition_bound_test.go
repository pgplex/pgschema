package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// partitionBoundIR builds an IR with a partitioned parent and a single child
// partition whose attachment/bound is given.
func partitionBoundIR(child *ir.Table) *ir.IR {
	parent := &ir.Table{
		Schema: "public", Name: "events", IsPartitioned: true,
		PartitionKey: "RANGE (id)",
		Columns:      []*ir.Column{{Name: "id", DataType: "integer"}},
	}
	return &ir.IR{Schemas: map[string]*ir.Schema{
		"public": {Name: "public", Tables: map[string]*ir.Table{
			"events":   parent,
			child.Name: child,
		}},
	}}
}

func boundSQL(t *testing.T, oldIR, newIR *ir.IR) string {
	t.Helper()
	var stmts []string
	for _, change := range GenerateMigration(oldIR, newIR, "public") {
		for _, s := range change.Statements {
			stmts = append(stmts, s.SQL)
		}
	}
	return strings.Join(stmts, "\n")
}

func childTable(bound string) *ir.Table {
	return &ir.Table{
		Schema: "public", Name: "events_early",
		PartitionOf: "events", PartitionOfSchema: "public", PartitionBound: bound,
		Columns: []*ir.Column{{Name: "id", DataType: "integer"}},
	}
}

// A changed bound must produce a plan that re-attaches the partition, not an
// empty plan. (#639)
func TestPartitionBoundChangeReattaches(t *testing.T) {
	oldIR := partitionBoundIR(childTable("FOR VALUES FROM (0) TO (10)"))
	newIR := partitionBoundIR(childTable("FOR VALUES FROM (0) TO (20)"))

	sql := boundSQL(t, oldIR, newIR)
	if strings.TrimSpace(sql) == "" {
		t.Fatal("changed partition bound produced an empty plan")
	}
	if !strings.Contains(sql, "DETACH PARTITION") {
		t.Fatalf("expected DETACH PARTITION, got:\n%s", sql)
	}
	if !strings.Contains(sql, "ATTACH PARTITION") || !strings.Contains(sql, "FOR VALUES FROM (0) TO (20)") {
		t.Fatalf("expected re-ATTACH with the new bound, got:\n%s", sql)
	}
}

// A partition that is removed from the desired schema must be detached. (#639)
func TestPartitionDetachEmitsStatement(t *testing.T) {
	oldIR := partitionBoundIR(childTable("FOR VALUES FROM (0) TO (10)"))
	standalone := childTable("")
	standalone.PartitionOf = ""
	standalone.PartitionOfSchema = ""
	newIR := partitionBoundIR(standalone)

	sql := boundSQL(t, oldIR, newIR)
	if strings.TrimSpace(sql) == "" {
		t.Fatal("detached partition produced an empty plan")
	}
	if !strings.Contains(sql, "DETACH PARTITION") {
		t.Fatalf("expected DETACH PARTITION, got:\n%s", sql)
	}
	if strings.Contains(sql, "ATTACH PARTITION") {
		t.Fatalf("detached partition should not be re-attached, got:\n%s", sql)
	}
}

// An unchanged partition must produce no plan. (#639 regression guard)
func TestPartitionBoundUnchangedNoPlan(t *testing.T) {
	oldIR := partitionBoundIR(childTable("FOR VALUES FROM (0) TO (10)"))
	newIR := partitionBoundIR(childTable("FOR VALUES FROM (0) TO (10)"))

	if sql := strings.TrimSpace(boundSQL(t, oldIR, newIR)); sql != "" {
		t.Fatalf("unchanged partition produced a plan:\n%s", sql)
	}
}
