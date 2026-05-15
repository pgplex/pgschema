package plan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/fingerprint"
)

func TestPlan_AddSchemaAndHasAnyChanges(t *testing.T) {
	p := NewPlan()

	// Empty plan has no changes
	if p.HasAnyChanges() {
		t.Error("empty plan should have no changes")
	}

	// Add a schema with no changes
	emptySP := NewSchemaPlan(nil)
	p.AddSchema("tenant_1", emptySP)
	if p.HasAnyChanges() {
		t.Error("plan with empty schema should have no changes")
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
	spWithChanges := NewSchemaPlan(diffs)
	p.AddSchema("tenant_2", spWithChanges)
	if !p.HasAnyChanges() {
		t.Error("plan with changes should report has changes")
	}
}

func TestPlan_SortedSchemaNames(t *testing.T) {
	p := NewPlan()
	p.AddSchema("tenant_c", NewSchemaPlan(nil))
	p.AddSchema("tenant_a", NewSchemaPlan(nil))
	p.AddSchema("tenant_b", NewSchemaPlan(nil))

	names := p.SortedSchemaNames()
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

func TestPlan_ToJSON_RoundTrip(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	p := NewPlan()

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
	sp := NewSchemaPlanWithFingerprint(diffs, fp)
	p.AddSchema("tenant_1", sp)

	// Add empty schema
	p.AddSchema("tenant_2", NewSchemaPlan(nil))

	// Serialize
	jsonStr, err := p.ToJSON()
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
	loaded, err := FromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if len(loaded.Schemas) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(loaded.Schemas))
	}

	// Verify tenant_1 has changes
	sp1, ok := loaded.Schemas["tenant_1"]
	if !ok {
		t.Fatal("missing tenant_1 schema")
	}
	if !sp1.HasAnyChanges() {
		t.Error("tenant_1 should have changes")
	}
	if sp1.SourceFingerprint == nil || sp1.SourceFingerprint.Hash != "abc123" {
		t.Error("tenant_1 fingerprint not preserved")
	}

	// Verify tenant_2 has no changes
	sp2, ok := loaded.Schemas["tenant_2"]
	if !ok {
		t.Fatal("missing tenant_2 schema")
	}
	if sp2.HasAnyChanges() {
		t.Error("tenant_2 should have no changes")
	}

	// Verify version fields are populated from parent
	if loaded.Version != p.Version {
		t.Errorf("version = %q, want %q", loaded.Version, p.Version)
	}
	if loaded.PgschemaVersion != p.PgschemaVersion {
		t.Errorf("pgschema_version = %q, want %q", loaded.PgschemaVersion, p.PgschemaVersion)
	}
}

func TestPlan_SchemaEntry_ExcludesTopLevelFields(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	p := NewPlan()
	p.AddSchema("test_schema", NewSchemaPlan(nil))

	jsonStr, err := p.ToJSON()
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

func TestFromJSON_ValidPlan(t *testing.T) {
	planJSON := `{
		"version": "1.0.0",
		"pgschema_version": "1.9.0",
		"created_at": "2025-01-01T00:00:00Z",
		"schemas": {
			"tenant_1": {
				"groups": []
			}
		}
	}`

	loaded, err := FromJSON([]byte(planJSON))
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}
	if len(loaded.Schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(loaded.Schemas))
	}
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	_, err := FromJSON([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPlan_SummaryString(t *testing.T) {
	p := NewPlan()

	// Empty
	s := p.SummaryString()
	if s != "Summary: 0 schemas inspected, 0 with changes" {
		t.Errorf("unexpected summary: %s", s)
	}

	// With schemas
	p.AddSchema("s1", NewSchemaPlan(nil))
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
	p.AddSchema("s2", NewSchemaPlan(diffs))

	s = p.SummaryString()
	if s != "Summary: 2 schemas inspected, 1 with changes" {
		t.Errorf("unexpected summary: %s", s)
	}
}

func TestPlan_HumanColored_MultiSchema(t *testing.T) {
	p := NewPlan()
	p.AddSchema("schema_a", NewSchemaPlan(nil))
	p.AddSchema("schema_b", NewSchemaPlan(nil))

	output := p.HumanColored(false)

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

func TestPlan_ToSQL_MultiSchema(t *testing.T) {
	p := NewPlan()
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
	p.AddSchema("s1", NewSchemaPlan(diffs))
	p.AddSchema("s2", NewSchemaPlan(nil))

	sql := p.ToSQL(SQLFormatRaw)

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

func TestPlan_CreatedAt_UsesTestTime(t *testing.T) {
	t.Setenv("PGSCHEMA_TEST_TIME", "2024-06-15T12:00:00Z")
	p := NewPlan()

	expected, _ := time.Parse(time.RFC3339, "2024-06-15T12:00:00Z")
	if !p.CreatedAt.Equal(expected) {
		t.Errorf("created_at = %v, want %v", p.CreatedAt, expected)
	}
}
