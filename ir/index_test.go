package ir

import (
	"encoding/json"
	"testing"
)

func TestPartitionInspectionMetadataNotSerialized(t *testing.T) {
	index := &Index{Schema: "public", Table: "events", Name: "event_lookup"}
	table := &Table{Schema: "public", Name: "events", Indexes: map[string]*Index{index.Name: index}}
	before, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	index.AttachedPartitions = []IndexRef{{Schema: "public", Name: "ignored_child"}}
	table.AllPartitionChildren = []TableDependency{{Schema: "public", Name: "ignored_partition"}}
	after, err := json.Marshal(table)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Inspection metadata changed schema serialization:\nbefore: %s\nafter: %s", before, after)
	}
}
