package plan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/fingerprint"
)

func TestMultiPlan_AddSchemaAndHasAnyChanges(t *testing.T) {
	mp := NewMultiPlan()

	// Empty multi-plan has no changes
	if mp.HasAnyChanges() {
		t.Error("empty multi-plan should have no changes")
	}

	// Add a schema with no changes
	emptyPlan := NewPlan(nil)
	mp.AddSchema("tenant_1", emptyPlan)
	if mp.HasAnyChanges() {
		t.Error("multi-plan with empty schema should have no changes")
	}

	// Add a schema with changes
	diffs := []diff.Diff{
		{
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.users",
			Statements: []diff.SQLStatement{
				{SQL: "CREATE TABLE users (id integer);"},
			},
		},
	}
	planWithChanges := NewPlan(diffs)
	mp.AddSchema("tenant_2", planWithChanges)
	if !mp.HasAnyChanges() {
		t.Error("multi-plan with changes should report has changes")
	}
}

func TestMultiPlan_SortedSchemaNames(t *testing.T) {
	mp := NewMultiPlan()
	mp.AddSchema("tenant_c", NewPlan(nil))
	mp.AddSchema("tenant_a", NewPlan(nil))
	mp.AddSchema("tenant_b", NewPlan(nil))

	names := mp.SortedSchemaNames()
	expected := []string{"tenant_a", "tenant_b", "tenant_c"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestMultiPlan_ToJSON_RoundTrip(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	mp := NewMultiPlan()

	// Add schema with fingerprint and changes
	fp := &fingerprint.SchemaFingerprint{Hash: "abc123"}
	diffs := []diff.Diff{
		{
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.users",
			Statements: []diff.SQLStatement{
				{SQL: "CREATE TABLE users (id integer);"},
			},
		},
	}
	p := NewPlanWithFingerprint(diffs, fp)
	mp.AddSchema("tenant_1", p)

	// Add empty schema
	mp.AddSchema("tenant_2", NewPlan(nil))

	// Serialize
	jsonStr, err := mp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify JSON structure
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Must have "schemas" key
	if _, ok := raw["schemas"]; !ok {
		t.Error("JSON should have 'schemas' key")
	}
	// Must have version fields at top level
	if _, ok := raw["version"]; !ok {
		t.Error("JSON should have 'version' key")
	}
	if _, ok := raw["pgschema_version"]; !ok {
		t.Error("JSON should have 'pgschema_version' key")
	}
	// Must NOT have "groups" at top level
	if _, ok := raw["groups"]; ok {
		t.Error("JSON should NOT have 'groups' key at top level")
	}

	// Deserialize
	loaded, err := MultiPlanFromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("MultiPlanFromJSON failed: %v", err)
	}

	if len(loaded.Schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(loaded.Schemas))
	}

	// Verify tenant_1 has changes
	entry1, ok := loaded.Schemas["tenant_1"]
	if !ok {
		t.Fatal("missing tenant_1 schema")
	}
	if !entry1.Plan.HasAnyChanges() {
		t.Error("tenant_1 should have changes")
	}
	if entry1.Plan.SourceFingerprint == nil || entry1.Plan.SourceFingerprint.Hash != "abc123" {
		t.Error("tenant_1 fingerprint not preserved")
	}

	// Verify tenant_2 has no changes
	entry2, ok := loaded.Schemas["tenant_2"]
	if !ok {
		t.Fatal("missing tenant_2 schema")
	}
	if entry2.Plan.HasAnyChanges() {
		t.Error("tenant_2 should have no changes")
	}

	// Verify version fields are populated from parent
	if loaded.Version != mp.Version {
		t.Errorf("version = %q, want %q", loaded.Version, mp.Version)
	}
	if loaded.PgschemaVersion != mp.PgschemaVersion {
		t.Errorf("pgschema_version = %q, want %q", loaded.PgschemaVersion, mp.PgschemaVersion)
	}
}

func TestMultiPlan_SchemaEntry_ExcludesTopLevelFields(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	mp := NewMultiPlan()
	mp.AddSchema("test_schema", NewPlan(nil))

	jsonStr, err := mp.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse the schemas section
	var parsed struct {
		Schemas map[string]json.RawMessage `json:"schemas"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	schemaJSON := string(parsed.Schemas["test_schema"])

	// Schema entry should NOT contain version, pgschema_version, or created_at
	if strings.Contains(schemaJSON, `"version"`) {
		t.Error("schema entry should not contain 'version'")
	}
	if strings.Contains(schemaJSON, `"pgschema_version"`) {
		t.Error("schema entry should not contain 'pgschema_version'")
	}
	if strings.Contains(schemaJSON, `"created_at"`) {
		t.Error("schema entry should not contain 'created_at'")
	}

	// Should contain "groups"
	if !strings.Contains(schemaJSON, `"groups"`) {
		t.Error("schema entry should contain 'groups'")
	}
}

func TestLoadPlanFile_DetectsMultiPlan(t *testing.T) {
	multiPlanJSON := `{
		"version": "1.0.0",
		"pgschema_version": "1.9.0",
		"created_at": "2025-01-01T00:00:00Z",
		"schemas": {
			"tenant_1": {
				"groups": []
			}
		}
	}`

	loaded, err := LoadPlanFile([]byte(multiPlanJSON))
	if err != nil {
		t.Fatalf("LoadPlanFile failed: %v", err)
	}

	mp, ok := loaded.(*MultiPlan)
	if !ok {
		t.Fatalf("expected *MultiPlan, got %T", loaded)
	}
	if len(mp.Schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(mp.Schemas))
	}
}

func TestLoadPlanFile_DetectsSinglePlan(t *testing.T) {
	singlePlanJSON := `{
		"version": "1.0.0",
		"pgschema_version": "1.9.0",
		"created_at": "2025-01-01T00:00:00Z",
		"groups": [
			{
				"steps": [
					{
						"sql": "CREATE TABLE users (id integer);",
						"type": "table",
						"operation": "create",
						"path": "public.users"
					}
				]
			}
		]
	}`

	loaded, err := LoadPlanFile([]byte(singlePlanJSON))
	if err != nil {
		t.Fatalf("LoadPlanFile failed: %v", err)
	}

	p, ok := loaded.(*Plan)
	if !ok {
		t.Fatalf("expected *Plan, got %T", loaded)
	}
	if !p.HasAnyChanges() {
		t.Error("plan should have changes")
	}
}

func TestLoadPlanFile_InvalidJSON(t *testing.T) {
	_, err := LoadPlanFile([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestMultiPlan_SummaryString(t *testing.T) {
	mp := NewMultiPlan()

	// Empty
	s := mp.SummaryString()
	if s != "Summary: 0 schemas inspected, 0 with changes" {
		t.Errorf("unexpected summary: %s", s)
	}

	// With schemas
	mp.AddSchema("s1", NewPlan(nil))
	diffs := []diff.Diff{
		{
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.t",
			Statements: []diff.SQLStatement{
				{SQL: "CREATE TABLE t();"},
			},
		},
	}
	mp.AddSchema("s2", NewPlan(diffs))

	s = mp.SummaryString()
	if s != "Summary: 2 schemas inspected, 1 with changes" {
		t.Errorf("unexpected summary: %s", s)
	}
}

func TestMultiPlan_HumanColored(t *testing.T) {
	mp := NewMultiPlan()
	mp.AddSchema("schema_a", NewPlan(nil))
	mp.AddSchema("schema_b", NewPlan(nil))

	output := mp.HumanColored(false)

	// Should contain schema headers in sorted order
	idxA := strings.Index(output, "Schema: schema_a")
	idxB := strings.Index(output, "Schema: schema_b")
	if idxA == -1 {
		t.Error("output should contain 'Schema: schema_a'")
	}
	if idxB == -1 {
		t.Error("output should contain 'Schema: schema_b'")
	}
	if idxA >= idxB {
		t.Error("schema_a should appear before schema_b")
	}
}

func TestMultiPlan_ToSQL(t *testing.T) {
	mp := NewMultiPlan()
	diffs := []diff.Diff{
		{
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.t",
			Statements: []diff.SQLStatement{
				{SQL: "CREATE TABLE t (id int)"},
			},
		},
	}
	mp.AddSchema("s1", NewPlan(diffs))
	mp.AddSchema("s2", NewPlan(nil))

	sql := mp.ToSQL(SQLFormatRaw)

	// Should contain header for s1 (has SQL)
	if !strings.Contains(sql, "-- Schema: s1") {
		t.Error("should contain schema header for s1")
	}
	if !strings.Contains(sql, "CREATE TABLE t (id int)") {
		t.Error("should contain SQL for s1")
	}
	// s2 has no SQL, should not have header
	if strings.Contains(sql, "-- Schema: s2") {
		t.Error("should not contain schema header for s2 (no SQL)")
	}
}

func TestMultiPlan_CreatedAt_UsesTestTime(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2024-06-15T12:00:00Z")
	mp := NewMultiPlan()

	expected, _ := time.Parse(time.RFC3339, "2024-06-15T12:00:00Z")
	if !mp.CreatedAt.Equal(expected) {
		t.Errorf("created_at = %v, want %v", mp.CreatedAt, expected)
	}
}
