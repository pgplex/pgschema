package dump

// Dump Integration Tests
// These comprehensive integration tests verify the entire dump workflow by comparing
// schema representations from two different sources:
// 1. Database inspection (pgdump.sql → database → dump command → schema output)
// 2. Expected output verification (comparing actual vs expected schema dumps)
// This ensures our pgschema output accurately represents the original database schema

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestDumpCommand_Employee(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "employee")
}

func TestDumpCommand_Sakila(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "sakila")
}

func TestDumpCommand_Bytebase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "bytebase")
}

func TestDumpCommand_TenantSchemas(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runTenantSchemaTest(t, "tenant")
}

func TestDumpCommand_Issue78ConstraintNotValidAndQuoting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_78_constraint_not_valid_and_quoting")
}

func TestDumpCommand_Issue80IndexNameQuote(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_80_index_name_quote")
}

func TestDumpCommand_Issue82ViewLogicExpr(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_82_view_logic_expr")
}

func TestDumpCommand_Issue83ExplicitConstraintName(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_83_explicit_constraint_name")
}

func TestDumpCommand_Issue125FunctionDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_125_function_default")
}

func TestDumpCommand_Issue133IndexSort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_133_index_sort")
}

func TestDumpCommand_Issue183GeneratedColumn(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_183_generated_column")
}

func TestDumpCommand_Issue275TruncatedFunctionGrants(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_275_truncated_function_grants")
}

func TestDumpCommand_Issue252FunctionSchemaQualifier(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_252_function_schema_qualifier")
}

func TestDumpCommand_Issue307ViewDependencyOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_307_view_dependency_order")
}

func TestDumpCommand_Issue320PlpgsqlReservedKeywordType(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_320_plpgsql_reserved_keyword_type")
}

func TestDumpCommand_Issue345ArrayCast(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_345_array_cast")
}

func TestDumpCommand_Issue396CheckConstraintIsNotNull(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_396_check_constraint_is_not_null")
}

func TestDumpCommand_Issue191FunctionProcedureOverload(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	runExactMatchTest(t, "issue_191_function_procedure_overload")
}

// Reproduces a bug where a column declared as `name` is dumped as `char[]`.
// The inspector classifies any base type with pg_type.typelem <> 0 as an array,
// but the `name` type has typelem = 18 (the OID of "char") despite not being an array.
func TestDumpCommand_NameTypeNotDumpedAsCharArray(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	_, err := conn.ExecContext(context.Background(), `CREATE TABLE pgschema_name_repro (n name);`)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	output, err := ExecuteDump(&DumpConfig{
		Host:     host,
		Port:     port,
		DB:       dbname,
		User:     user,
		Password: password,
		Schema:   "public",
	})
	if err != nil {
		t.Fatalf("Dump command failed: %v", err)
	}

	if strings.Contains(output, "char[]") {
		t.Errorf("Dump output should not contain char[] for a name column.\nOutput:\n%s", output)
	}
	if !strings.Contains(output, "n name") {
		t.Errorf("Dump output should contain `n name` column declaration.\nOutput:\n%s", output)
	}
}

func TestDumpCommand_Issue318CrossSchemaComment(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup PostgreSQL
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	// Connect to database
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Read and execute the setup SQL that creates two schemas with same-named tables
	setupPath := "../../testdata/dump/issue_318_cross_schema_comment/setup.sql"
	setupContent, err := os.ReadFile(setupPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", setupPath, err)
	}

	_, err = conn.ExecContext(context.Background(), string(setupContent))
	if err != nil {
		t.Fatalf("Failed to execute setup.sql: %v", err)
	}

	// Dump each schema and verify comments are correctly attributed
	tests := []struct {
		schema       string
		tableComment string
		colComment   string
	}{
		{"alpha", "Alpha account table", "Alpha account name"},
		{"beta", "Beta account table", "Beta account name"},
	}

	for _, tc := range tests {
		t.Run(tc.schema, func(t *testing.T) {
			config := &DumpConfig{
				Host:      host,
				Port:      port,
				DB:        dbname,
				User:      user,
				Password:  password,
				Schema:    tc.schema,
				MultiFile: false,
				File:      "",
			}

			output, err := ExecuteDump(config)
			if err != nil {
				t.Fatalf("Dump command failed for schema %s: %v", tc.schema, err)
			}

			// Verify table comment
			expectedTableComment := fmt.Sprintf("COMMENT ON TABLE account IS '%s';", tc.tableComment)
			if !strings.Contains(output, expectedTableComment) {
				t.Errorf("Schema %s: expected table comment %q not found in output:\n%s", tc.schema, expectedTableComment, output)
			}

			// Verify column comment
			expectedColComment := fmt.Sprintf("COMMENT ON COLUMN account.name IS '%s';", tc.colComment)
			if !strings.Contains(output, expectedColComment) {
				t.Errorf("Schema %s: expected column comment %q not found in output:\n%s", tc.schema, expectedColComment, output)
			}
		})
	}
}

func TestDumpCommand_Issue307MultiFileViewDependencyOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup PostgreSQL
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	// Connect to database
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Read and execute the pgdump.sql file
	pgdumpPath := "../../testdata/dump/issue_307_view_dependency_order/pgdump.sql"
	pgdumpContent, err := os.ReadFile(pgdumpPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", pgdumpPath, err)
	}

	// Execute the SQL to create the schema
	_, err = conn.ExecContext(context.Background(), string(pgdumpContent))
	if err != nil {
		t.Fatalf("Failed to execute pgdump.sql: %v", err)
	}

	// Create temp directory for multi-file output
	tmpDir := t.TempDir()
	outputPath := tmpDir + "/schema.sql"

	// Create dump configuration for multi-file mode
	config := &DumpConfig{
		Host:      host,
		Port:      port,
		DB:        dbname,
		User:      user,
		Password:  password,
		Schema:    "public",
		MultiFile: true,
		File:      outputPath,
	}

	// Execute pgschema dump in multi-file mode
	_, err = ExecuteDump(config)
	if err != nil {
		t.Fatalf("Dump command failed: %v", err)
	}

	// Read the main schema file to check include order
	mainContent, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read main file: %v", err)
	}

	// Parse include directives to check view ordering
	lines := strings.Split(string(mainContent), "\n")
	var viewIncludes []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, `\i views/`) {
			viewIncludes = append(viewIncludes, trimmed)
		}
	}

	// Verify we have both view includes
	if len(viewIncludes) != 2 {
		t.Fatalf("Expected 2 view includes, got %d: %v", len(viewIncludes), viewIncludes)
	}

	// item_summary must come before dashboard because dashboard depends on item_summary
	// (even though "dashboard" sorts before "item_summary" alphabetically)
	if viewIncludes[0] != `\i views/item_summary.sql` {
		t.Errorf("Expected first view include to be item_summary (dependency), got: %s", viewIncludes[0])
	}
	if viewIncludes[1] != `\i views/dashboard.sql` {
		t.Errorf("Expected second view include to be dashboard (depends on item_summary), got: %s", viewIncludes[1])
	}
}

func TestDumpCommand_Issue323SupabaseDefaultPrivilegeFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Verifies that dump filters out default privileges for roles the current
	// user is not a member of. Simulates Supabase where 'postgres' is not a
	// superuser and has no membership in 'supabase_admin'.

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	conn, host, port, dbname, _, _ := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	ctx := context.Background()

	majorVersion, err := testutil.GetMajorVersion(conn)
	if err != nil {
		t.Fatalf("Failed to detect PostgreSQL version: %v", err)
	}
	testutil.ShouldSkipTest(t, t.Name(), majorVersion)

	// Create system_admin (simulating supabase_admin) and limited_user
	// (simulating the connecting postgres user). limited_user is NOT a member
	// of system_admin.
	_, err = conn.ExecContext(ctx, fmt.Sprintf(`
		CREATE ROLE system_admin;
		CREATE ROLE app_user;
		CREATE ROLE limited_user LOGIN PASSWORD 'limitedpass';
		GRANT CONNECT ON DATABASE %s TO limited_user;
		SET ROLE system_admin;
		ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO app_user;
		RESET ROLE;
	`, dbname))
	if err != nil {
		t.Fatalf("Failed to set up roles and privileges: %v", err)
	}

	// Dump as limited_user (non-superuser, not a member of system_admin).
	output, err := ExecuteDump(&DumpConfig{
		Host:     host,
		Port:     port,
		DB:       dbname,
		User:     "limited_user",
		Password: "limitedpass",
		Schema:   "public",
	})
	if err != nil {
		t.Fatalf("Dump command failed: %v", err)
	}

	if strings.Contains(output, "system_admin") {
		t.Errorf("Dump as limited_user should not include system_admin's default privileges\nActual output:\n%s", output)
	}
}

func runExactMatchTest(t *testing.T, testDataDir string) {
	runExactMatchTestWithContext(t, context.Background(), testDataDir)
}

func runExactMatchTestWithContext(t *testing.T, ctx context.Context, testDataDir string) {
	// Setup PostgreSQL
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	// Connect to database
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Detect PostgreSQL version and skip tests if needed
	majorVersion, err := testutil.GetMajorVersion(conn)
	if err != nil {
		t.Fatalf("Failed to detect PostgreSQL version: %v", err)
	}

	// Check if this test should be skipped for this PostgreSQL version
	// If skipped, ShouldSkipTest will call t.Skipf() and stop execution
	testutil.ShouldSkipTest(t, t.Name(), majorVersion)

	// Read and execute the pgdump.sql file
	pgdumpPath := fmt.Sprintf("../../testdata/dump/%s/pgdump.sql", testDataDir)
	pgdumpContent, err := os.ReadFile(pgdumpPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", pgdumpPath, err)
	}

	// Execute the SQL to create the schema
	_, err = conn.ExecContext(ctx, string(pgdumpContent))
	if err != nil {
		t.Fatalf("Failed to execute pgdump.sql: %v", err)
	}

	// Create dump configuration
	config := &DumpConfig{
		Host:      host,
		Port:      port,
		DB:        dbname,
		User:      user,
		Password:  password,
		Schema:    "public",
		MultiFile: false,
		File:      "",
	}

	// Execute pgschema dump
	actualOutput, err := ExecuteDump(config)
	if err != nil {
		t.Fatalf("Dump command failed: %v", err)
	}

	// Read expected output
	expectedPath := fmt.Sprintf("../../testdata/dump/%s/pgschema.sql", testDataDir)
	expectedContent, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", expectedPath, err)
	}
	expectedOutput := string(expectedContent)

	// Use shared comparison function
	compareSchemaOutputs(t, actualOutput, expectedOutput, testDataDir)
}

func runTenantSchemaTest(t *testing.T, testDataDir string) {
	// Setup PostgreSQL
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	// Connect to database
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Read the tenant SQL that will be loaded into all schemas
	tenantSQL, err := os.ReadFile(fmt.Sprintf("../../testdata/dump/%s/tenant.sql", testDataDir))
	if err != nil {
		t.Fatalf("Failed to read tenant.sql: %v", err)
	}

	// Load utility functions (if util.sql exists)
	utilPath := fmt.Sprintf("../../testdata/dump/%s/util.sql", testDataDir)
	if utilSQL, err := os.ReadFile(utilPath); err == nil {
		_, err = conn.Exec(string(utilSQL))
		if err != nil {
			t.Fatalf("Failed to load utility functions from util.sql: %v", err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("Failed to read util.sql: %v", err)
	}

	// Create two tenant schemas (public already exists)
	schemas := []string{"public", "tenant1", "tenant2"}
	for _, schema := range schemas[1:] { // Skip public as it already exists
		_, err = conn.Exec(fmt.Sprintf("CREATE SCHEMA %s", schema))
		if err != nil {
			t.Fatalf("Failed to create schema %s: %v", schema, err)
		}
	}

	// Load the tenant SQL into all three schemas
	for _, schema := range schemas {
		// Set search path to target schema only
		quotedSchema := ir.QuoteIdentifier(schema)
		_, err = conn.Exec(fmt.Sprintf("SET search_path TO %s", quotedSchema))
		if err != nil {
			t.Fatalf("Failed to set search path to %s: %v", schema, err)
		}

		// Execute the SQL - objects will be created in the target schema
		_, err = conn.Exec(string(tenantSQL))
		if err != nil {
			t.Fatalf("Failed to load SQL into schema %s: %v", schema, err)
		}
	}

	// Dump all three schemas using pgschema dump command
	var dumps []string
	for _, schemaName := range schemas {
		// Create dump configuration for this schema
		config := &DumpConfig{
			Host:      host,
			Port:      port,
			DB:        dbname,
			User:      user,
			Password:  password,
			Schema:    schemaName,
			MultiFile: false,
			File:      "",
		}

		// Execute pgschema dump
		actualOutput, err := ExecuteDump(config)
		if err != nil {
			t.Fatalf("Dump command failed for schema %s: %v", schemaName, err)
		}
		dumps = append(dumps, actualOutput)
	}

	// Read expected output
	expectedBytes, err := os.ReadFile(fmt.Sprintf("../../testdata/dump/%s/pgschema.sql", testDataDir))
	if err != nil {
		t.Fatalf("Failed to read expected output: %v", err)
	}
	expected := string(expectedBytes)

	// Compare all dumps against expected output
	for i, dump := range dumps {
		schemaName := schemas[i]

		// Use shared comparison function
		compareSchemaOutputs(t, dump, expected, fmt.Sprintf("%s_%s", testDataDir, schemaName))
	}

	// Also compare all dumps with each other - they should be identical
	for i := 1; i < len(dumps); i++ {
		compareSchemaOutputs(t, dumps[0], dumps[i], fmt.Sprintf("%s_%s_vs_%s", testDataDir, schemas[0], schemas[i]))
	}
}

// normalizeSchemaOutput removes version-specific lines for comparison.
// This allows comparing dumps across different PostgreSQL versions.
func normalizeSchemaOutput(output string) string {
	lines := strings.Split(output, "\n")
	var normalizedLines []string

	for _, line := range lines {
		// Skip version-related lines
		if strings.Contains(line, "-- Dumped by pgschema version") ||
			strings.Contains(line, "-- Dumped from database version") {
			continue
		}
		normalizedLines = append(normalizedLines, line)
	}

	return strings.Join(normalizedLines, "\n")
}

// compareSchemaOutputs compares actual and expected schema outputs
func compareSchemaOutputs(t *testing.T, actualOutput, expectedOutput string, testName string) {
	// Normalize both outputs to ignore version differences
	normalizedActual := normalizeSchemaOutput(actualOutput)
	normalizedExpected := normalizeSchemaOutput(expectedOutput)

	// Compare the normalized outputs
	if normalizedActual != normalizedExpected {
		t.Errorf("Output does not match for %s", testName)
		t.Logf("Total lines - Actual: %d, Expected: %d", len(strings.Split(actualOutput, "\n")), len(strings.Split(expectedOutput, "\n")))

		// Write actual output to file for debugging only when test fails
		actualFilename := fmt.Sprintf("%s_actual.sql", testName)
		os.WriteFile(actualFilename, []byte(actualOutput), 0644)

		expectedFilename := fmt.Sprintf("%s_expected.sql", testName)
		os.WriteFile(expectedFilename, []byte(expectedOutput), 0644)
		t.Logf("Outputs written to %s and %s for debugging", actualFilename, expectedFilename)

		// Find and show first difference
		actualLines := strings.Split(normalizedActual, "\n")
		expectedLines := strings.Split(normalizedExpected, "\n")

		for i := 0; i < len(actualLines) && i < len(expectedLines); i++ {
			if actualLines[i] != expectedLines[i] {
				t.Errorf("First difference at line %d:\nActual:   %s\nExpected: %s",
					i+1, actualLines[i], expectedLines[i])
				break
			}
		}

		if len(actualLines) != len(expectedLines) {
			t.Errorf("Different number of lines - Actual: %d, Expected: %d",
				len(actualLines), len(expectedLines))
		}
	}
}
