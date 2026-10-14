package ir

import (
	"encoding/json"
	"testing"
)

func TestPartitionInspectionMetadataNotSerialized(t *testing.T) {
	index := &Index{Schema: "public", Table: "events", Name: "event_lookup"}
	table := &Table{Schema: "public", Name: "events", Indexes: map[string]*Index{index.Name: index}}
	constraint := &Constraint{Schema: "public", Table: "events", Name: "unique_constraint", Type: ConstraintTypeUnique}
	table.Constraints = map[string]*Constraint{constraint.Name: constraint}
	before, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	index.AttachedPartitions = []IndexRef{{Schema: "public", Name: "ignored_child"}}
	constraint.BackingIndex = &IndexRef{Schema: "public", Name: "unique_constraint"}
	constraint.CatalogDefinition = "UNIQUE (id) INCLUDE (payload)"
	constraint.CatalogComment = "local key"
	table.AllPartitionChildren = []TableDependency{{Schema: "public", Name: "ignored_partition"}}
	table.InheritedConstraintIndexes = []InheritedConstraintIndex{{
		Definition: "UNIQUE (id) INCLUDE (payload)", Comment: "inherited key",
		Index: IndexRef{Schema: "public", Name: "renamed_inherited"}, ConstraintName: "renamed_inherited", ConstraintType: ConstraintTypeUnique,
		ParentIndex: IndexRef{Schema: "public", Name: "origin"}, ParentConstraint: ConstraintRef{Schema: "public", Table: "parent", Name: "origin"},
	}}
	after, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Inspection metadata changed schema serialization:\nbefore: %s\nafter: %s", before, after)
	}
}
