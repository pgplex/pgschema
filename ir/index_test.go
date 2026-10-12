package ir

import (
	"encoding/json"
	"testing"
)

func TestIndexAttachedPartitionsAreInspectionMetadata(t *testing.T) {
	index := &Index{Schema: "public", Table: "events", Name: "event_lookup"}
	before, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	index.AttachedPartitions = []IndexRef{{Schema: "public", Name: "ignored_child"}}
	after, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("Inspection metadata changed index serialization:\nbefore: %s\nafter: %s", before, after)
	}
}
