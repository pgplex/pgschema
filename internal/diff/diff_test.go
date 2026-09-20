package diff

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
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

// buildSQLFromSteps builds a SQL string from collected plan diffs
func buildSQLFromSteps(diffs []Diff) string {
	var sqlOutput strings.Builder

	for i, step := range diffs {
		// Add all SQL statements for this step
		for j, stmt := range step.Statements {
			// Handle regular SQL statements (directives are handled at plan level, not diff level)
			sqlOutput.WriteString(stmt.SQL)
			sqlOutput.WriteString("\n")

			// Add separator between statements within a step
			if j < len(step.Statements)-1 {
				sqlOutput.WriteString("\n")
			}
		}

		// Add separator between steps (but not after the last one)
		if i < len(diffs)-1 {
			sqlOutput.WriteString("\n")
		}
	}

	return sqlOutput.String()
}

// TestDiffFromFiles runs file-based diff tests from testdata directory.
// It walks through the testdata/diff directory structure looking for test cases
// that contain old.sql, new.sql, and plan.sql files. For each test case,
// it parses the old and new schemas, computes the diff, generates migration SQL,
// and compares it against the expected plan.
//
// Test filtering can be controlled using the PGSCHEMA_TEST_FILTER environment variable:
//
// Examples:
//
//	# Run all tests under alter_table/ (directory prefix with slash)
//	PGSCHEMA_TEST_FILTER="alter_table/" go test -v ./internal/diff
//
//	# Run tests under alter_table/ that start with "add_column"
//	PGSCHEMA_TEST_FILTER="alter_table/add_column" go test -v ./internal/diff
//
//	# Run a specific test
//	PGSCHEMA_TEST_FILTER="alter_table/add_column_with_fk" go test -v ./internal/diff
func TestDiffFromFiles(t *testing.T) {
	testdataDir := filepath.Join("../../testdata/diff")

	// Check if testdata directory exists
	if _, err := os.Stat(testdataDir); os.IsNotExist(err) {
		t.Skip("testdata directory does not exist, skipping file-based tests")
		return
	}

	// Get test filter from environment variable
	testFilter := os.Getenv("PGSCHEMA_TEST_FILTER")

	// Track number of test cases found
	testCount := 0

	// Walk through all statement type directories (e.g., create_table, alter_table)
	err := filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root testdata directory and statement type directories
		if path == testdataDir || strings.Count(path, string(os.PathSeparator)) <= strings.Count(testdataDir, string(os.PathSeparator))+1 {
			return nil
		}

		// Only process directories that contain test cases
		if !info.IsDir() {
			return nil
		}

		// Check if this directory contains the required test files
		oldFile := filepath.Join(path, "old.sql")
		newFile := filepath.Join(path, "new.sql")
		diffFile := filepath.Join(path, "diff.sql")

		// Extract test name from path
		relPath, _ := filepath.Rel(testdataDir, path)
		testName := strings.ReplaceAll(relPath, string(os.PathSeparator), "_")

		// Apply test filter if provided
		if testFilter != "" && !matchesFilter(relPath, testFilter) {
			return nil
		}

		// Increment test counter
		testCount++

		// Run the test case as a subtest
		t.Run(testName, func(t *testing.T) {
			runFileBasedDiffTest(t, oldFile, newFile, diffFile, testName)
		})

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk testdata directory: %v", err)
	}

	// Check if filter was provided but no tests matched
	if testFilter != "" && testCount == 0 {
		t.Fatalf("No test cases found matching filter: %s", testFilter)
	}
}

// runFileBasedDiffTest executes a single file-based diff test
func runFileBasedDiffTest(t *testing.T, oldFile, newFile, diffFile, testName string) {
	// Detect PostgreSQL version and skip tests if needed
	// Get connection from shared postgres instance
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer conn.Close()

	majorVersion, err := testutil.GetMajorVersion(conn)
	if err != nil {
		t.Fatalf("Failed to detect PostgreSQL version: %v", err)
	}

	// If skipped, ShouldSkipTest will call t.Skipf() and stop execution
	testutil.ShouldSkipTest(t, testName, majorVersion)

	// Read optional setup.sql (for cross-schema setup, extension types, etc.)
	// This will be passed to ParseSQLToIRWithSetup to run AFTER schema recreation
	// so that extensions in public schema aren't dropped (issue #197)
	var setupSQL string
	setupFile := filepath.Join(filepath.Dir(oldFile), "setup.sql")
	if _, err := os.Stat(setupFile); err == nil {
		setupContent, err := os.ReadFile(setupFile)
		if err != nil {
			t.Fatalf("Failed to read setup.sql: %v", err)
		}
		setupSQL = string(setupContent)
	}

	// Read old DDL
	oldDDL, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatalf("Failed to read old.sql: %v", err)
	}

	// Read new DDL
	newDDL, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("Failed to read new.sql: %v", err)
	}

	// Read expected plan
	expectedPlan, err := os.ReadFile(diffFile)
	if err != nil {
		t.Fatalf("Failed to read plan.sql: %v", err)
	}

	// Parse DDL to IR with optional setup SQL
	// setup.sql runs after schema recreation so extensions in public schema aren't dropped
	oldIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, string(oldDDL), "public", setupSQL)
	newIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, string(newDDL), "public", setupSQL)

	// Run diff
	diffs := GenerateMigration(oldIR, newIR, "public")

	// Generate migration SQL
	actualPlan := buildSQLFromSteps(diffs)

	// Normalize whitespace for comparison
	expected := normalizeSQL(string(expectedPlan))
	actual := normalizeSQL(actualPlan)

	if actual != expected {
		t.Errorf("Migration SQL mismatch:\nExpected:\n%s\n\nActual:\n%s", expected, actual)
	}
}

// matchesFilter checks if a test should run based on the filter pattern
// It supports:
// - Directory prefix with slash: "alter_table/" (matches all tests under alter_table/)
// - Specific test pattern: "alter_table/add_column_with_fk" (matches specific test under alter_table/)
// - Prefix pattern: "alter_table/add_column" (matches tests under alter_table/ that start with add_column)
func matchesFilter(relPath, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	// Handle directory prefix patterns with trailing slash
	if strings.HasSuffix(filter, "/") {
		// "alter_table/" matches "alter_table/add_column_with_fk"
		return strings.HasPrefix(relPath+"/", filter)
	}

	// Handle patterns with slash (both specific tests and prefix patterns)
	if strings.Contains(filter, "/") {
		// "alter_table/add_column_with_fk" matches "alter_table/add_column_with_fk"
		// "alter_table/add_column" matches "alter_table/add_column_with_fk"
		return strings.HasPrefix(relPath, filter)
	}

	// Fallback: check if filter is a substring of the path
	return strings.Contains(relPath, filter)
}

// normalizeSQL normalizes SQL for comparison by trimming whitespace and removing empty lines
func normalizeSQL(sql string) string {
	lines := strings.Split(sql, "\n")
	var normalizedLines []string

	for _, line := range lines {
		// Preserve leading whitespace (indentation) but trim trailing whitespace
		trimmed := strings.TrimRight(line, " \t")
		if trimmed != "" {
			normalizedLines = append(normalizedLines, trimmed)
		}
	}

	return strings.Join(normalizedLines, "\n")
}
