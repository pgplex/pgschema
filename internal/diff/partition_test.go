package diff

import (
	"slices"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func constraintColumns(names ...string) []*ir.ConstraintColumn {
	columns := make([]*ir.ConstraintColumn, len(names))
	for i, name := range names {
		columns[i] = &ir.ConstraintColumn{Name: name, Position: i + 1}
	}
	return columns
}

// ATTACH PARTITION adopts an existing child constraint by its columns alone and
// keeps the child's own deferrability and referential actions, so a copy that
// differs from the parent's constraint in those must not be kept. (#639)
func TestParentSuppliesConstraint(t *testing.T) {
	parentPK := &ir.Constraint{Name: "events_pkey", Type: ir.ConstraintTypePrimaryKey, Columns: constraintColumns("id", "at")}
	parentFK := &ir.Constraint{
		Name: "events_account_id_fkey", Type: ir.ConstraintTypeForeignKey,
		Columns: constraintColumns("account_id"), ReferencedSchema: "public", ReferencedTable: "accounts",
		ReferencedColumns: constraintColumns("id"), DeleteRule: "SET NULL", DeleteSetColumns: []string{"account_id"}, UpdateRule: "NO ACTION",
	}
	parent := &ir.Table{Schema: "public", Name: "events", Constraints: map[string]*ir.Constraint{parentPK.Name: parentPK, parentFK.Name: parentFK}}

	childPK := *parentPK
	childPK.Name = "events_2026_01_pkey"
	deferrablePK := childPK
	deferrablePK.Deferrable = true
	includedPK := childPK
	includedPK.IncludeColumns = []string{"payload"}
	childFK := *parentFK
	childFK.Name = "events_2026_01_account_id_fkey"
	setNullAllFK := childFK
	setNullAllFK.DeleteSetColumns = nil
	checkCopy := &ir.Constraint{Name: "events_amount_check", Type: ir.ConstraintTypeCheck, CheckClause: "CHECK (amount > 0)"}

	tests := []struct {
		name       string
		constraint *ir.Constraint
		want       bool
	}{
		{"primary key copy", &childPK, true},
		{"primary key with different deferrability", &deferrablePK, false},
		{"primary key with different included columns", &includedPK, false},
		{"foreign key copy", &childFK, true},
		{"foreign key with different SET NULL column list", &setNullAllFK, false},
		{"check constraint", checkCopy, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parentSuppliesConstraint(effectiveConstraints(nil, parent), tt.constraint); got != tt.want {
				t.Errorf("parentSuppliesConstraint() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A detached partition keeps its parent's PK/UNIQUE/FK copies as local
// constraints under the names the catalog gave them, so the old state used for
// the diff must list them; the inspected table itself is left untouched. (#639)
func TestWithConstraintsRetainedByDetach(t *testing.T) {
	parentPK := &ir.Constraint{Schema: "public", Table: "events", Name: "events_pkey", Type: ir.ConstraintTypePrimaryKey, Columns: constraintColumns("id", "at"), IncludeColumns: []string{"payload", "select"}, Comment: "parent comment"}
	parentFK := &ir.Constraint{Schema: "public", Table: "events", Name: "events_account_id_fkey", Type: ir.ConstraintTypeForeignKey, Columns: constraintColumns("account_id"), ReferencedTable: "accounts", ReferencedColumns: constraintColumns("id")}
	parent := &ir.Table{Schema: "public", Name: "events", Constraints: map[string]*ir.Constraint{parentPK.Name: parentPK, parentFK.Name: parentFK}}
	check := &ir.Constraint{Schema: "public", Table: "events_2026_01", Name: "events_amount_check", Type: ir.ConstraintTypeCheck}
	child := &ir.Table{
		Schema: "public", Name: "events_2026_01", PartitionOf: "events",
		Constraints:          map[string]*ir.Constraint{check.Name: check},
		InheritedConstraints: map[string]string{"events_pkey": "events_2026_01_pkey", "events_account_id_fkey": "events_account_id_fkey", "gone": "gone_copy"},
	}

	tables := map[string]*ir.Table{"public.events": parent, "public.events_2026_01": child}
	got := withConstraintsRetainedByDetach(child, tables)

	if len(child.Constraints) != 1 {
		t.Fatalf("inspected table modified: %d constraints", len(child.Constraints))
	}
	if len(got.Constraints) != 3 {
		t.Fatalf("got %d constraints, want 3", len(got.Constraints))
	}
	pk := got.Constraints["events_2026_01_pkey"]
	if pk == nil || pk.Type != ir.ConstraintTypePrimaryKey || pk.Table != "events_2026_01" || pk.Comment != "" || !slices.Equal(pk.IncludeColumns, parentPK.IncludeColumns) {
		t.Errorf("retained primary key = %+v", pk)
	}
	if fk := got.Constraints["events_account_id_fkey"]; fk == nil || fk.ReferencedTable != "accounts" || fk.Table != "events_2026_01" {
		t.Errorf("retained foreign key = %+v", fk)
	}
	if got.Constraints[check.Name] != check {
		t.Error("child's own constraint not carried over")
	}
	if same := withConstraintsRetainedByDetach(child, map[string]*ir.Table{}); same != child {
		t.Error("expected the table itself when the parent is unknown")
	}
}

// In a root → partition → sub-partition hierarchy the middle table's IR holds
// neither its own copy of the root's primary key nor, therefore, the definition
// the leaf's copy derives from: it has to be resolved up the chain. (#639)
func TestResolveInheritedConstraintNested(t *testing.T) {
	rootPK := &ir.Constraint{Schema: "public", Table: "metrics", Name: "metrics_pkey", Type: ir.ConstraintTypePrimaryKey, Columns: constraintColumns("id", "bucket")}
	root := &ir.Table{Schema: "public", Name: "metrics", IsPartitioned: true, Constraints: map[string]*ir.Constraint{rootPK.Name: rootPK}}
	mid := &ir.Table{Schema: "public", Name: "metrics_low", IsPartitioned: true, PartitionOf: "metrics", InheritedConstraints: map[string]string{"metrics_pkey": "metrics_low_pkey"}}
	leaf := &ir.Table{Schema: "public", Name: "metrics_low_b", PartitionOf: "metrics_low", InheritedConstraints: map[string]string{"metrics_low_pkey": "metrics_low_b_pkey"}}
	tables := map[string]*ir.Table{"public.metrics": root, "public.metrics_low": mid, "public.metrics_low_b": leaf}

	if got := resolveInheritedConstraint(tables, mid, "metrics_low_pkey"); got != rootPK {
		t.Errorf("resolveInheritedConstraint() = %+v, want the root primary key", got)
	}
	if got := resolveInheritedConstraint(tables, mid, "unknown"); got != nil {
		t.Errorf("resolveInheritedConstraint() = %+v for an unknown copy", got)
	}
	retained := withConstraintsRetainedByDetach(leaf, tables).Constraints["metrics_low_b_pkey"]
	if retained == nil || retained.Type != ir.ConstraintTypePrimaryKey || retained.Table != "metrics_low_b" {
		t.Errorf("leaf's retained primary key = %+v", retained)
	}
	supplied := effectiveConstraints(tables, mid)
	if len(supplied) != 1 || supplied[0] != rootPK {
		t.Errorf("effectiveConstraints(mid) = %+v, want the root primary key", supplied)
	}
}
