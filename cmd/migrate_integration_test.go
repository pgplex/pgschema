package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/cmd/apply"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
)

var (
	generate = flag.Bool("generate", false, "generate expected test output files instead of comparing")
	// sharedEmbeddedPG is a shared embedded PostgreSQL instance used across all integration tests
	// to significantly improve test performance by avoiding repeated startup/teardown
	sharedEmbeddedPG *postgres.EmbeddedPostgres
)

// TestMain sets up shared resources for all tests in this package
func TestMain(m *testing.M) {
	// Parse flags
	flag.Parse()

	// Create shared embedded postgres instance for all integration tests
	// This dramatically improves test performance (from ~60s to ~10s per test)
	sharedEmbeddedPG = testutil.SetupPostgres(nil)
	defer sharedEmbeddedPG.Stop()

	m.Run()
}

// TestPlanAndApply tests the complete CLI (plan and apply) workflow using test cases
// from testdata/diff/. This test exercises the full end-to-end CLI commands that
// users will execute, providing comprehensive validation of the plan and apply workflow.
//
// The test performs these actions for each test case:
// 1. Apply old.sql to database → initialize starting state
// 2. Run plan command with new.sql → generate and validate all output formats
// 3. Apply migration using apply command → execute the planned changes
// 4. Verify idempotency → re-running plan should produce no changes
//
// Test filtering can be controlled using the PGSCHEMA_TEST_FILTER environment variable:
//
// Examples:
//
//	# Run all tests under create_table/ (directory prefix with slash)
//	PGSCHEMA_TEST_FILTER="create_table/" go test -v ./cmd -run TestPlanAndApply
//
//	# Run tests under create_table/ that start with "add_column"
//	PGSCHEMA_TEST_FILTER="create_table/add_column" go test -v ./cmd -run TestPlanAndApply
//
//	# Run a specific test
//	PGSCHEMA_TEST_FILTER="create_table/add_column_identity" go test -v ./cmd -run TestPlanAndApply
//
//	# Run all migrate tests
//	PGSCHEMA_TEST_FILTER="migrate/" go test -v ./cmd -run TestPlanAndApply
func TestPlanAndApply(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	testDataRoot := "../testdata/diff"

	// Start a single PostgreSQL container for all test cases
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Get test filter from environment variable
	testFilter := os.Getenv("PGSCHEMA_TEST_FILTER")

	// Collect all test cases first
	var testCases []testCase
	err := filepath.Walk(testDataRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip if not a directory or if it's the root or category directories
		if !info.IsDir() || path == testDataRoot {
			return nil
		}

		// Skip category directories (e.g., create_table/, create_index/) - only process leaf directories
		relPath, _ := filepath.Rel(testDataRoot, path)
		pathDepth := len(strings.Split(relPath, string(filepath.Separator)))
		if pathDepth == 1 {
			return nil // Skip category directories
		}

		// Check if this directory contains the required test files
		oldFile := filepath.Join(path, "old.sql")
		newFile := filepath.Join(path, "new.sql")
		setupFile := filepath.Join(path, "setup.sql")
		planSQLFile := filepath.Join(path, "plan.sql")
		planJSONFile := filepath.Join(path, "plan.json")
		planTXTFile := filepath.Join(path, "plan.txt")

		// Check for required input files (always required)
		if _, err := os.Stat(oldFile); os.IsNotExist(err) {
			return fmt.Errorf("missing required file: %s", oldFile)
		}
		if _, err := os.Stat(newFile); os.IsNotExist(err) {
			return fmt.Errorf("missing required file: %s", newFile)
		}

		// Check for output files when not generating
		if !*generate {
			if _, err := os.Stat(planSQLFile); os.IsNotExist(err) {
				return fmt.Errorf("missing required file: %s (use --generate to create)", planSQLFile)
			}
			if _, err := os.Stat(planJSONFile); os.IsNotExist(err) {
				return fmt.Errorf("missing required file: %s (use --generate to create)", planJSONFile)
			}
			if _, err := os.Stat(planTXTFile); os.IsNotExist(err) {
				return fmt.Errorf("missing required file: %s (use --generate to create)", planTXTFile)
			}
		}

		// Apply test filter if provided
		if testFilter != "" && !matchesFilter(relPath, testFilter) {
			return nil
		}

		// Get relative path for test name
		testName := strings.ReplaceAll(relPath, string(filepath.Separator), "_")

		testCases = append(testCases, testCase{
			name:         testName,
			oldFile:      oldFile,
			newFile:      newFile,
			setupFile:    setupFile,
			planSQLFile:  planSQLFile,
			planJSONFile: planJSONFile,
			planTXTFile:  planTXTFile,
		})

		return nil
	})

	if err != nil {
		t.Fatalf("Failed to walk test data directory: %v", err)
	}

	// Check if filter was provided but no tests matched
	if testFilter != "" && len(testCases) == 0 {
		t.Fatalf("No test cases found matching filter: %s", testFilter)
	}

	// Run all test cases using the shared container
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runPlanAndApplyTest(t, ctx, container, tc)
		})
	}
}

type testCase struct {
	name         string
	oldFile      string
	newFile      string
	setupFile    string
	planSQLFile  string
	planJSONFile string
	planTXTFile  string
}

// runPlanAndApplyTest executes a single plan and apply test case with test-specific database
func runPlanAndApplyTest(t *testing.T, ctx context.Context, container *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, tc testCase) {
	// Detect PostgreSQL version and skip tests if needed
	majorVersion, err := testutil.GetMajorVersion(container.Conn)
	if err != nil {
		t.Fatalf("Failed to detect PostgreSQL version: %v", err)
	}

	// Check if this test should be skipped for this PostgreSQL version
	// If skipped, ShouldSkipTest will call t.Skipf() and stop execution
	testutil.ShouldSkipTest(t, tc.name, majorVersion)

	containerHost := container.Host
	portMapped := container.Port
	// Create a unique database name for this test case (replace invalid chars)
	dbName := "test_" + strings.ReplaceAll(strings.ReplaceAll(tc.name, "/", "_"), "-", "_")
	// PostgreSQL identifiers are limited to 63 characters
	if len(dbName) > 63 {
		dbName = dbName[:63]
	}

	// Create test-specific database
	if err := createDatabase(ctx, containerHost, portMapped, dbName); err != nil {
		t.Fatalf("Failed to create test database %s: %v", dbName, err)
	}

	// STEP 0: Execute optional setup.sql (for cross-schema setup, extension types, etc.)
	if _, err := os.Stat(tc.setupFile); err == nil {
		setupContent, err := os.ReadFile(tc.setupFile)
		if err != nil {
			t.Fatalf("Failed to read setup.sql: %v", err)
		}
		if len(strings.TrimSpace(string(setupContent))) > 0 {
			// Execute setup.sql to target database
			if err := executeSQL(ctx, containerHost, portMapped, dbName, string(setupContent)); err != nil {
				t.Fatalf("Failed to execute setup.sql to target database: %v", err)
			}

			// Also execute setup.sql to shared embedded postgres instance
			// This ensures both databases have the setup objects available
			embeddedConn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedEmbeddedPG)
			defer embeddedConn.Close()
			if _, err := embeddedConn.ExecContext(ctx, string(setupContent)); err != nil {
				t.Fatalf("Failed to execute setup.sql to embedded postgres: %v", err)
			}
		}
	}

	// STEP 1: Apply old.sql to initialize database state
	oldContent, err := os.ReadFile(tc.oldFile)
	if err != nil {
		t.Fatalf("Failed to read %s: %v", tc.oldFile, err)
	}

	// Execute old.sql if it has content
	if len(strings.TrimSpace(string(oldContent))) > 0 {
		if err := executeSQL(ctx, containerHost, portMapped, dbName, string(oldContent)); err != nil {
			t.Fatalf("Failed to execute old.sql: %v", err)
		}
	}

	// STEP 2: Test plan command with new.sql as target
	// Note: setup.sql has already been executed to both databases in STEP 0,
	// so we only need to use new.sql for plan and apply operations
	testPlanOutputs(t, container, dbName, tc.newFile, tc.planSQLFile, tc.planJSONFile, tc.planTXTFile)

	if !*generate {
		// STEP 3: Apply the migration using apply command
		err = applySchemaChanges(containerHost, portMapped, dbName, container.User, container.Password, "public", tc.newFile)
		if err != nil {
			t.Fatalf("Failed to apply schema changes using pgschema apply: %v", err)
		}

		// STEP 4: Test idempotency - plan should produce no changes
		secondPlanOutput, err := generatePlanSQLFormatted(containerHost, portMapped, dbName, container.User, container.Password, "public", tc.newFile)
		if err != nil {
			t.Fatalf("Failed to generate plan SQL for idempotency check: %v", err)
		}

		if secondPlanOutput != "" {
			t.Errorf("Expected no changes when applying schema twice, but got SQL output:\n%s", secondPlanOutput)
		}
	}
}

// testPlanOutputs tests all plan output formats against expected files
func testPlanOutputs(t *testing.T, container *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, dbName, schemaFile, planSQLFile, planJSONFile, planTXTFile string) {
	containerHost := container.Host
	portMapped := container.Port
	// Set fixed timestamp for generate mode to ensure deterministic output
	if *generate {
		os.Setenv("PGSCHEMA_TEST_TIME", "1970-01-01T00:00:00Z")
		defer os.Unsetenv("PGSCHEMA_TEST_TIME")
	}
	// Test SQL format
	sqlFormattedOutput, err := generatePlanSQLFormatted(containerHost, portMapped, dbName, container.User, container.Password, "public", schemaFile)
	if err != nil {
		t.Fatalf("Failed to generate plan SQL formatted output: %v", err)
	}

	if *generate {
		// Generate mode: write actual output to expected file
		actualSQLStr := strings.ReplaceAll(sqlFormattedOutput, "\r\n", "\n")
		err := os.WriteFile(planSQLFile, []byte(actualSQLStr), 0644)
		if err != nil {
			t.Fatalf("Failed to write expected SQL output file %s: %v", planSQLFile, err)
		}
		t.Logf("Generated expected SQL output file %s", planSQLFile)
	} else {
		// Compare mode: compare with expected file (always required)
		expectedSQL, err := os.ReadFile(planSQLFile)
		if err != nil {
			t.Fatalf("Failed to read expected SQL output file %s: %v", planSQLFile, err)
		}

		// Compare SQL output
		expectedSQLStr := strings.ReplaceAll(string(expectedSQL), "\r\n", "\n")
		actualSQLStr := strings.ReplaceAll(sqlFormattedOutput, "\r\n", "\n")
		if actualSQLStr != expectedSQLStr {
			t.Errorf("SQL output mismatch.\nExpected:\n%s\n\nActual:\n%s", expectedSQLStr, actualSQLStr)
			// Write actual output to file for easier comparison
			actualFile := strings.Replace(planSQLFile, ".sql", "_actual.sql", 1)
			os.WriteFile(actualFile, []byte(actualSQLStr), 0644)
			t.Logf("Actual SQL output written to %s", actualFile)
		}
	}

	// Test human-readable format
	humanOutput, err := generatePlanHuman(containerHost, portMapped, dbName, container.User, container.Password, "public", schemaFile)
	if err != nil {
		t.Fatalf("Failed to generate plan human output: %v", err)
	}

	if *generate {
		// Generate mode: write actual output to expected file
		actualHumanStr := strings.ReplaceAll(humanOutput, "\r\n", "\n")
		err := os.WriteFile(planTXTFile, []byte(actualHumanStr), 0644)
		if err != nil {
			t.Fatalf("Failed to write expected human output file %s: %v", planTXTFile, err)
		}
		t.Logf("Generated expected human output file %s", planTXTFile)
	} else {
		// Compare mode: compare with expected file (always required)
		expectedHuman, err := os.ReadFile(planTXTFile)
		if err != nil {
			t.Fatalf("Failed to read expected human output file %s: %v", planTXTFile, err)
		}

		// Compare human output (normalize line endings and trim)
		expectedHumanStr := strings.TrimSpace(strings.ReplaceAll(string(expectedHuman), "\r\n", "\n"))
		actualHumanStr := strings.TrimSpace(strings.ReplaceAll(humanOutput, "\r\n", "\n"))
		if actualHumanStr != expectedHumanStr {
			t.Errorf("Human output mismatch.\nExpected:\n%s\n\nActual:\n%s", expectedHumanStr, actualHumanStr)
			// Write actual output to file for easier comparison
			actualFile := strings.Replace(planTXTFile, ".txt", "_actual.txt", 1)
			os.WriteFile(actualFile, []byte(actualHumanStr), 0644)
			t.Logf("Actual human output written to %s", actualFile)
		}
	}

	// Test JSON format
	jsonOutput, err := generatePlanJSON(containerHost, portMapped, dbName, container.User, container.Password, "public", schemaFile)
	if err != nil {
		t.Fatalf("Failed to generate plan JSON output: %v", err)
	}

	if *generate {
		// Generate mode: write actual output to expected file
		err := os.WriteFile(planJSONFile, []byte(jsonOutput), 0644)
		if err != nil {
			t.Fatalf("Failed to write expected JSON output file %s: %v", planJSONFile, err)
		}
		t.Logf("Generated expected JSON output file %s", planJSONFile)
	} else {
		// Compare mode: compare with expected file (always required)
		expectedJSONBytes, err := os.ReadFile(planJSONFile)
		if err != nil {
			t.Fatalf("Failed to read expected JSON output file %s: %v", planJSONFile, err)
		}

		// Parse both JSON structures
		var expectedJSON, actualJSON map[string]interface{}

		if err := json.Unmarshal(expectedJSONBytes, &expectedJSON); err != nil {
			t.Fatalf("Failed to parse expected JSON: %v", err)
		}

		if err := json.Unmarshal([]byte(jsonOutput), &actualJSON); err != nil {
			t.Fatalf("Failed to parse actual JSON: %v. JSON output length: %d, content: %q", err, len(jsonOutput), jsonOutput)
		}

		// Compare JSON using go-cmp, ignoring dynamic fields
		ignoreFields := cmp.FilterPath(func(p cmp.Path) bool {
			// Get the last element of the path
			if len(p) == 0 {
				return false
			}
			last := p[len(p)-1]
			if mf, ok := last.(cmp.MapIndex); ok {
				key := fmt.Sprintf("%v", mf.Key().Interface())
				// Match field names
				return key == "created_at" || key == "pgschema_version"
			}
			return false
		}, cmp.Ignore())

		if diff := cmp.Diff(expectedJSON, actualJSON, ignoreFields); diff != "" {
			t.Errorf("JSON plan mismatch (-want +got):\n%s", diff)
		}
	}
}

// applySchemaChanges applies schema changes using the ApplyMigration API directly
func applySchemaChanges(host string, port int, database, user, password, schema, schemaFile string) error {
	// Create apply configuration
	config := &apply.ApplyConfig{
		Host:            host,
		Port:            port,
		DB:              database,
		User:            user,
		Password:        password,
		Schema:          schema,
		File:            schemaFile,
		AutoApprove:     true,
		NoColor:         true,
		Quiet:           true, // Suppress plan display and progress messages in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration API directly with shared embedded postgres
	return apply.ApplyMigration(config, sharedEmbeddedPG)
}

// generatePlanOutput generates plan output by calling GeneratePlan directly with shared embedded postgres
func generatePlanOutput(host string, port int, database, user, password, schema, schemaFile, outputFlag string, extraArgs ...string) (string, error) {
	// Create plan configuration with shared embedded postgres for performance
	config := &planCmd.PlanConfig{
		Host:            host,
		Port:            port,
		DB:              database,
		User:            user,
		Password:        password,
		Schema:          schema,
		File:            schemaFile,
		ApplicationName: "pgschema",
	}

	// Generate the plan (reuse shared embedded postgres for performance)
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	if err != nil {
		return "", err
	}

	// Format output based on the requested format
	var output string
	switch outputFlag {
	case "--output-human":
		// Check for --no-color in extraArgs
		useColor := true
		for _, arg := range extraArgs {
			if arg == "--no-color" {
				useColor = false
				break
			}
		}
		output = migrationPlan.HumanColored(useColor)
	case "--output-json":
		// Check for --debug in extraArgs
		debug := false
		for _, arg := range extraArgs {
			if arg == "--debug" {
				debug = true
				break
			}
		}
		jsonOutput, err := migrationPlan.ToJSONWithDebug(debug)
		if err != nil {
			return "", fmt.Errorf("failed to generate JSON output: %w", err)
		}
		output = jsonOutput + "\n"
	case "--output-sql":
		output = migrationPlan.ToSQL(plan.SQLFormatRaw)
	default:
		return "", fmt.Errorf("unknown output format: %s", outputFlag)
	}

	return output, nil
}

// generatePlanHuman generates plan human-readable output
func generatePlanHuman(host string, port int, database, user, password, schema, schemaFile string) (string, error) {
	return generatePlanOutput(host, port, database, user, password, schema, schemaFile, "--output-human", "--no-color")
}

// generatePlanJSON generates plan JSON output
func generatePlanJSON(host string, port int, database, user, password, schema, schemaFile string) (string, error) {
	return generatePlanOutput(host, port, database, user, password, schema, schemaFile, "--output-json")
}

// generatePlanSQLFormatted generates plan SQL output
func generatePlanSQLFormatted(host string, port int, database, user, password, schema, schemaFile string) (string, error) {
	return generatePlanOutput(host, port, database, user, password, schema, schemaFile, "--output-sql")
}

// matchesFilter checks if a relative path matches the given filter pattern
// createDatabase creates a test-specific database using the shared container
func createDatabase(ctx context.Context, host string, port int, dbName string) error {
	// Connect to postgres database to create the test database
	dsn := fmt.Sprintf("postgres://testuser:testpass@%s:%d/postgres?sslmode=disable", host, port)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres database: %v", err)
	}
	defer db.Close()

	// Create the test database
	_, err = db.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		return fmt.Errorf("failed to create database %s: %v", dbName, err)
	}

	return nil
}

// executeSQL executes SQL statements in the specified database
func executeSQL(ctx context.Context, host string, port int, dbName string, sqlContent string) error {
	// Connect to the specific test database
	dsn := fmt.Sprintf("postgres://testuser:testpass@%s:%d/%s?sslmode=disable", host, port, dbName)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database %s: %v", dbName, err)
	}
	defer db.Close()

	// Execute the SQL
	_, err = db.ExecContext(ctx, sqlContent)
	if err != nil {
		return fmt.Errorf("failed to execute SQL in database %s: %v", dbName, err)
	}

	return nil
}

// matchesFilter checks if a relative path matches the given filter pattern
func matchesFilter(relPath, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}

	// Handle directory prefix patterns with trailing slash
	if strings.HasSuffix(filter, "/") {
		// "create_table/" matches "create_table/add_column_identity"
		return strings.HasPrefix(relPath+"/", filter)
	}

	// Handle patterns with slash (both specific tests and prefix patterns)
	if strings.Contains(filter, "/") {
		// "create_table/add_column_identity" matches "create_table/add_column_identity"
		// "create_table/add_column" matches "create_table/add_column_identity"
		return strings.HasPrefix(relPath, filter)
	}

	// Fallback: check if filter is a substring of the path
	return strings.Contains(relPath, filter)
}
