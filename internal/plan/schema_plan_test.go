package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

// sharedTestPostgres is the shared embedded postgres instance for all tests in this package
var sharedTestPostgres *postgres.EmbeddedPostgres

// TestMain sets up shared resources for all tests in this package
func TestMain(m *testing.M) {
	// Create shared embedded postgres for all tests to dramatically improve performance
	sharedTestPostgres = testutil.SetupPostgres(nil)
	defer sharedTestPostgres.Stop()

	m.Run()
}

// discoverTestDataVersions discovers available test data versions in the testdata directory
func discoverTestDataVersions(testdataDir string) ([]string, error) {
	entries, err := os.ReadDir(testdataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read testdata directory: %w", err)
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			// Check if the directory contains a plan.json file
			planFile := filepath.Join(testdataDir, entry.Name(), "plan.json")
			if _, err := os.Stat(planFile); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}
	// Sort versions to ensure deterministic test execution order
	sort.Strings(versions)
	return versions, nil
}

// parseSQL is a helper function to convert SQL string to IR for tests
// Uses embedded PostgreSQL to ensure tests use the same code path as production
func parseSQL(t *testing.T, sql string) *ir.IR {
	t.Helper()
	return testutil.ParseSQLToIR(t, sharedTestPostgres, sql, "public")
}

func TestSchemaPlanSummary(t *testing.T) {
	oldSQL := `CREATE TABLE users (
		id integer NOT NULL
	);`

	newSQL := `CREATE TABLE users (
		id integer NOT NULL,
		name text NOT NULL
	);
	CREATE TABLE posts (
		id integer NOT NULL,
		title text NOT NULL
	);`

	oldIR := parseSQL(t, oldSQL)
	newIR := parseSQL(t, newSQL)
	diffs := diff.GenerateMigration(oldIR, newIR, "public")

	sp := NewSchemaPlan(diffs)
	summary := sp.HumanColored(false)

	// Debug: print the summary to see what it looks like
	t.Logf("Summary output:\n%s", summary)

	if !strings.Contains(summary, "1 to add") {
		t.Error("Summary should mention 1 resource to add")
	}

	if !strings.Contains(summary, "1 to modify") {
		t.Error("Summary should mention 1 resource to modify")
	}

	// The colored output doesn't show "0 to drop" when there are no drops
	if strings.Contains(summary, "to drop") && !strings.Contains(summary, "1 to add, 1 to modify") {
		t.Error("Summary should not mention drops when there are none")
	}
}

func TestPlanJSONRoundTrip(t *testing.T) {
	testDataDir := "../../testdata/diff/migrate"

	// Discover available test data versions dynamically
	versions, err := discoverTestDataVersions(testDataDir)
	if err != nil {
		t.Fatalf("Failed to discover test data versions: %v", err)
	}

	if len(versions) == 0 {
		t.Skip("No test data versions found")
	}

	for _, version := range versions {
		t.Run(fmt.Sprintf("version_%s", version), func(t *testing.T) {
			planFilePath := filepath.Join(testDataDir, version, "plan.json")

			// Read the original plan.json file
			originalJSON, err := os.ReadFile(planFilePath)
			if err != nil {
				t.Fatalf("Failed to read %s: %v", planFilePath, err)
			}

			// First FromJSON: Load plan from JSON
			plan1, err := FromJSON(originalJSON)
			if err != nil {
				t.Fatalf("Failed to parse JSON from %s: %v", planFilePath, err)
			}

			// Check if original JSON has source fields to determine debug mode
			hasSourceFields := strings.Contains(string(originalJSON), `"source":`)

			// First ToJSON: Convert plan back to JSON with same debug mode as original
			json1, err := plan1.ToJSONWithDebug(hasSourceFields)
			if err != nil {
				t.Fatalf("Failed to convert plan to JSON (first): %v", err)
			}

			// Compare original JSON with first round-trip JSON
			// Parse both JSON strings into maps to compare structure
			var originalMap, roundTripMap map[string]interface{}
			if err := json.Unmarshal(originalJSON, &originalMap); err != nil {
				t.Fatalf("Failed to unmarshal original JSON: %v", err)
			}
			if err := json.Unmarshal([]byte(json1), &roundTripMap); err != nil {
				t.Fatalf("Failed to unmarshal round-trip JSON: %v", err)
			}

			// Use go-cmp to show detailed differences
			if diff := cmp.Diff(originalMap, roundTripMap); diff != "" {
				t.Errorf("JSON round-trip failed for %s: mismatch (-original +roundtrip):\n%s", version, diff)
			}

			// Second round-trip: FromJSON -> ToJSON again
			// This should produce identical string output
			plan2, err := FromJSON([]byte(json1))
			if err != nil {
				t.Fatalf("Failed to parse JSON from round-trip: %v", err)
			}

			json2, err := plan2.ToJSONWithDebug(hasSourceFields)
			if err != nil {
				t.Fatalf("Failed to convert plan to JSON (second): %v", err)
			}

			// After first round-trip, subsequent round-trips should produce identical strings
			if json1 != json2 {
				t.Errorf("JSON not stable after first round-trip for %s", version)
				t.Logf("First round-trip length: %d", len(json1))
				t.Logf("Second round-trip length: %d", len(json2))

				// Show structural differences if any
				var map1, map2 map[string]interface{}
				json.Unmarshal([]byte(json1), &map1)
				json.Unmarshal([]byte(json2), &map2)
				if diff := cmp.Diff(map1, map2); diff != "" {
					t.Errorf("Structural difference in second round-trip (-first +second):\n%s", diff)
				}
			}
		})
	}
}

func TestSchemaPlanNoChanges(t *testing.T) {
	sql := `CREATE TABLE users (
                id integer NOT NULL
        );`

	oldIR := parseSQL(t, sql)
	newIR := parseSQL(t, sql)
	diffs := diff.GenerateMigration(oldIR, newIR, "public")

	sp := NewSchemaPlan(diffs)
	summary := strings.TrimSpace(sp.HumanColored(false))

	if summary != "No changes detected." {
		t.Errorf("expected %q, got %q", "No changes detected.", summary)
	}
}

func TestPlanJSONLoadedSummary(t *testing.T) {
	// Test that plans loaded from JSON can generate summaries using Steps metadata
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	// Create a plan with steps that have metadata
	p := NewPlan()
	p.AddSchema("public", &SchemaPlan{
		Groups: []ExecutionGroup{
			{
				Steps: []Step{
					{
						SQL:       "CREATE TABLE users (id serial primary key);",
						Type:      "table",
						Operation: "create",
						Path:      "public.users",
					},
					{
						SQL:       "ALTER TABLE posts ADD COLUMN title text;",
						Type:      "table.column",
						Operation: "create",
						Path:      "public.posts.title",
					},
				},
			},
		},
	})

	// Serialize to JSON (without SourceDiffs)
	jsonData, err := p.ToJSON()
	if err != nil {
		t.Fatalf("Failed to serialize plan to JSON: %v", err)
	}

	// Load plan from JSON
	loadedPlan, err := FromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to load plan from JSON: %v", err)
	}

	// Verify SourceDiffs is empty (as expected for JSON-loaded plans)
	loadedSP := loadedPlan.Schemas["public"]
	if len(loadedSP.SourceDiffs) != 0 {
		t.Errorf("Expected empty SourceDiffs, got %d", len(loadedSP.SourceDiffs))
	}

	// Generate summary - this should work using Steps metadata
	summary := loadedSP.HumanColored(false)

	// Verify summary contains expected information
	if !strings.Contains(summary, "1 to add") {
		t.Error("Summary should mention 1 resource to add")
	}

	if !strings.Contains(summary, "Tables:") {
		t.Error("Summary should contain Tables section")
	}

	if !strings.Contains(summary, "users") {
		t.Error("Summary should mention users table")
	}

	if strings.Contains(summary, "No changes detected") {
		t.Error("Summary should not say \"No changes detected\" when there are changes")
	}
}

func TestPlanDebugJSONRoundTrip(t *testing.T) {
	// Issue #305: Plans generated with --debug produce JSON that cannot be
	// deserialized by FromJSON() because the Diff.Source field is a Go interface
	// (DiffSource) that json.Unmarshal cannot reconstruct.
	t.Setenv("PGSCHEMA_TEST_TIME", "2025-01-01T00:00:00Z")

	oldSQL := `CREATE TABLE users (
		id integer NOT NULL
	);`

	newSQL := `CREATE TABLE users (
		id integer NOT NULL,
		name text NOT NULL
	);
	CREATE TABLE posts (
		id integer NOT NULL,
		title text NOT NULL
	);`

	oldIR := parseSQL(t, oldSQL)
	newIR := parseSQL(t, newSQL)
	diffs := diff.GenerateMigration(oldIR, newIR, "public")

	sp := NewSchemaPlan(diffs)
	p := NewPlan()
	p.AddSchema("public", sp)

	// Serialize with debug mode (includes SourceDiffs; Diff.Source is excluded via json:"-")
	debugJSON, err := p.ToJSONWithDebug(true)
	if err != nil {
		t.Fatalf("Failed to serialize plan with debug: %v", err)
	}

	// Deserialize - this should succeed
	loaded, err := FromJSON([]byte(debugJSON))
	if err != nil {
		t.Fatalf("Failed to deserialize debug plan JSON: %v", err)
	}

	// Verify debug mode actually included SourceDiffs
	loadedSP := loaded.Schemas["public"]
	if len(loadedSP.SourceDiffs) == 0 {
		t.Error("Debug plan should include SourceDiffs")
	}

	// Verify the loaded plan has valid groups and steps
	if len(loadedSP.Groups) == 0 {
		t.Error("Loaded plan should have at least one execution group")
	}

	// Re-serialize without debug and verify round-trip stability
	normalJSON, err := loaded.ToJSON()
	if err != nil {
		t.Fatalf("Failed to re-serialize loaded plan: %v", err)
	}

	loaded2, err := FromJSON([]byte(normalJSON))
	if err != nil {
		t.Fatalf("Failed to deserialize re-serialized plan: %v", err)
	}

	loadedSP2 := loaded2.Schemas["public"]
	if len(loadedSP2.Groups) != len(loadedSP.Groups) {
		t.Errorf("Group count mismatch: got %d, want %d", len(loadedSP2.Groups), len(loadedSP.Groups))
	}
}

// TestPlanSingleSchemaOmitsHeader verifies that single-schema plans
// render without a "Schema: ..." header line.
func TestPlanSingleSchemaOmitsHeader(t *testing.T) {
	p := NewPlan()
	p.AddSchema("public", NewSchemaPlan(nil))

	output := p.HumanColored(false)
	if strings.Contains(output, "Schema:") {
		t.Error("single-schema plan should not contain schema header")
	}
}

// ignore unused import warning for time
var _ = time.Now
