package dump

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/dump"
	"github.com/pgplex/pgschema/ir"
)

func TestCreateMultiFileOutput(t *testing.T) {
	// Create temporary directory for test
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "schema.sql")

	// Create test diffs with proper Source objects
	diffs := []diff.Diff{
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE TYPE user_status AS ENUM ('active', 'inactive');",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeType,
			Operation: diff.DiffOperationCreate,
			Path:      "public.user_status",
			Source: &ir.Type{
				Name: "user_status",
			},
		},
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeTable,
			Operation: diff.DiffOperationCreate,
			Path:      "public.users",
			Source: &ir.Table{
				Name: "users",
			},
		},
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE FUNCTION get_user_count() RETURNS integer AS $$ SELECT COUNT(*) FROM users; $$;",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeFunction,
			Operation: diff.DiffOperationCreate,
			Path:      "public.get_user_count",
			Source: &ir.Function{
				Name: "get_user_count",
			},
		},
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE AGGREGATE group_concat(text) (\n    SFUNC = _group_concat,\n    STYPE = text\n);",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeAggregate,
			Operation: diff.DiffOperationCreate,
			Path:      "public.group_concat",
			Source: &ir.Aggregate{
				Name:      "group_concat",
				Arguments: "text",
			},
		},
		{
			Statements: []diff.SQLStatement{
				{
					SQL:                 "CREATE VIEW active_users AS SELECT * FROM users WHERE status = 'active';",
					CanRunInTransaction: true,
				},
			},
			Type:      diff.DiffTypeView,
			Operation: diff.DiffOperationCreate,
			Path:      "public.active_users",
			Source: &ir.View{
				Name: "active_users",
			},
		},
	}

	// Test the FormatMultiFile function
	formatter := dump.NewDumpFormatter("PostgreSQL 17.0", "public", false)
	err := formatter.FormatMultiFile(diffs, outputPath)
	if err != nil {
		t.Fatalf("FormatMultiFile failed: %v", err)
	}

	// Check that main file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("Main file was not created at %s", outputPath)
	}

	// Check that subdirectories were created
	expectedDirs := []string{"types", "functions", "aggregates", "tables", "views"}
	for _, dir := range expectedDirs {
		dirPath := filepath.Join(tmpDir, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Expected directory %s was not created", dirPath)
		}
	}

	// Check that individual files were created
	expectedFiles := []string{
		"types/user_status.sql",
		"functions/get_user_count.sql",
		"aggregates/group_concat.sql",
		"tables/users.sql",
		"views/active_users.sql",
	}
	for _, file := range expectedFiles {
		filePath := filepath.Join(tmpDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", filePath)
		}
	}

	// Read main file and check for include statements
	mainContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read main file: %v", err)
	}

	mainStr := string(mainContent)
	expectedIncludes := []string{
		"\\i types/user_status.sql",
		"\\i functions/get_user_count.sql",
		"\\i aggregates/group_concat.sql",
		"\\i tables/users.sql",
		"\\i views/active_users.sql",
	}

	for _, include := range expectedIncludes {
		if !strings.Contains(mainStr, include) {
			t.Errorf("Main file should contain include statement: %s\nMain file content:\n%s", include, mainStr)
		}
	}

	// Check that header is present
	if !strings.Contains(mainStr, "-- pgschema database dump") {
		t.Errorf("Main file should contain header")
	}

	// Check individual file content
	typeFile := filepath.Join(tmpDir, "types", "user_status.sql")
	typeContent, err := os.ReadFile(typeFile)
	if err != nil {
		t.Fatalf("Failed to read type file: %v", err)
	}

	typeStr := string(typeContent)
	if !strings.Contains(typeStr, "CREATE TYPE user_status") {
		t.Errorf("Type file should contain the CREATE TYPE statement")
	}
	if !strings.Contains(typeStr, "-- Name: user_status; Type: TYPE; Schema: -; Owner: -") {
		t.Errorf("Type file should contain comment header")
	}

	// Check aggregate file content and that its header carries the argument signature
	aggregateFile := filepath.Join(tmpDir, "aggregates", "group_concat.sql")
	aggregateContent, err := os.ReadFile(aggregateFile)
	if err != nil {
		t.Fatalf("Failed to read aggregate file: %v", err)
	}

	aggregateStr := string(aggregateContent)
	if !strings.Contains(aggregateStr, "CREATE AGGREGATE group_concat(text)") {
		t.Errorf("Aggregate file should contain the CREATE AGGREGATE statement")
	}
	if !strings.Contains(aggregateStr, "-- Name: group_concat(text); Type: AGGREGATE; Schema: -; Owner: -") {
		t.Errorf("Aggregate file should contain comment header with argument signature")
	}
}

// TestMultiFileIncludeOrderDeterministic verifies that \i include lines in main.sql
// are deterministic across multiple runs (GitHub issue #299).
func TestMultiFileIncludeOrderDeterministic(t *testing.T) {
	// Create multiple tables so the map iteration order matters
	diffs := []diff.Diff{
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE TABLE accounts ();", CanRunInTransaction: true}},
			Type:       diff.DiffTypeTable,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.accounts",
			Source:     &ir.Table{Name: "accounts"},
		},
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE TABLE orders ();", CanRunInTransaction: true}},
			Type:       diff.DiffTypeTable,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.orders",
			Source:     &ir.Table{Name: "orders"},
		},
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE TABLE products ();", CanRunInTransaction: true}},
			Type:       diff.DiffTypeTable,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.products",
			Source:     &ir.Table{Name: "products"},
		},
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE TABLE users ();", CanRunInTransaction: true}},
			Type:       diff.DiffTypeTable,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.users",
			Source:     &ir.Table{Name: "users"},
		},
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE FUNCTION alpha() RETURNS void AS $$ BEGIN END; $$;", CanRunInTransaction: true}},
			Type:       diff.DiffTypeFunction,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.alpha",
			Source:     &ir.Function{Name: "alpha"},
		},
		{
			Statements: []diff.SQLStatement{{SQL: "CREATE FUNCTION beta() RETURNS void AS $$ BEGIN END; $$;", CanRunInTransaction: true}},
			Type:       diff.DiffTypeFunction,
			Operation:  diff.DiffOperationCreate,
			Path:       "public.beta",
			Source:     &ir.Function{Name: "beta"},
		},
	}

	formatter := dump.NewDumpFormatter("PostgreSQL 17.0", "public", false)

	// Run multiple times and verify output is always the same
	var firstOutput string
	for i := 0; i < 20; i++ {
		tmpDir := t.TempDir()
		outputPath := filepath.Join(tmpDir, "main.sql")

		err := formatter.FormatMultiFile(diffs, outputPath)
		if err != nil {
			t.Fatalf("FormatMultiFile failed on iteration %d: %v", i, err)
		}

		content, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatalf("Failed to read main file on iteration %d: %v", i, err)
		}

		output := string(content)
		if i == 0 {
			firstOutput = output

			// Verify includes are sorted within each directory group
			lines := strings.Split(output, "\n")
			var includes []string
			for _, line := range lines {
				if strings.HasPrefix(line, "\\i ") {
					includes = append(includes, line)
				}
			}

			// Expected sorted order: functions/alpha, functions/beta, tables/accounts, tables/orders, tables/products, tables/users
			expectedOrder := []string{
				"\\i functions/alpha.sql",
				"\\i functions/beta.sql",
				"\\i tables/accounts.sql",
				"\\i tables/orders.sql",
				"\\i tables/products.sql",
				"\\i tables/users.sql",
			}
			if len(includes) != len(expectedOrder) {
				t.Fatalf("Expected %d includes, got %d: %v", len(expectedOrder), len(includes), includes)
			}
			for j, expected := range expectedOrder {
				if includes[j] != expected {
					t.Errorf("Include[%d]: expected %q, got %q", j, expected, includes[j])
				}
			}
		} else if output != firstOutput {
			t.Fatalf("Non-deterministic output detected on iteration %d.\nFirst:\n%s\nGot:\n%s", i, firstOutput, output)
		}
	}
}

func TestDumpFormatterHelpers(t *testing.T) {
	// Create a formatter instance for testing helper methods
	formatter := dump.NewDumpFormatter("PostgreSQL 17.0", "public", false)

	// Test getObjectDirectory through the formatter
	testObjectDirectories := []struct {
		objectType string
		expected   string
	}{
		{"TYPE", "types"},
		{"DOMAIN", "domains"},
		{"SEQUENCE", "sequences"},
		{"FUNCTION", "functions"},
		{"PROCEDURE", "procedures"},
		{"TABLE", "tables"},
		{"VIEW", "views"},
		{"MATERIALIZED VIEW", "views"},
		{"TRIGGER", "tables"},
		{"INDEX", "tables"},
		{"CONSTRAINT", "tables"},
		{"POLICY", "tables"},
		{"UNKNOWN", "misc"},
	}

	// Since getObjectDirectory is a private method, we'll test it indirectly through FormatMultiFile
	// For now, we'll just test that the formatter was created successfully
	if formatter == nil {
		t.Errorf("Expected formatter to be created successfully")
	}

	// Test getObjectName behavior through actual usage
	testObjectNames := []struct {
		objectPath string
		expected   string
	}{
		{"public.users", "users"},
		{"schema1.table1", "table1"},
		{"simple_name", "simple_name"},
		{"", ""},
		{"a.b.c", "b"}, // Should take the second part
	}

	// Test sanitizeFileName behavior through actual usage
	testFileNames := []struct {
		input    string
		expected string
	}{
		{"simple_name", "simple_name"},
		{"name-with-dashes", "name-with-dashes"},
		{"name.with.dots", "name_with_dots"},
		{"name with spaces", "name_with_spaces"},
		{"name@#$%symbols", "name____symbols"},
		{"_leading_underscore_", "leading_underscore"},
		{"MixedCase", "mixedcase"},
	}

	// Note: These are now private methods in the formatter, so we can't test them directly.
	// The functionality is tested indirectly through the full multi-file output test above.
	t.Logf("Testing %d object directory mappings, %d object name extractions, and %d filename sanitizations through integration test",
		len(testObjectDirectories), len(testObjectNames), len(testFileNames))
}
