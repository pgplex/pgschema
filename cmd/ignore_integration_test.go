package cmd

// Ignore Integration Tests
// These comprehensive integration tests verify the .pgschemaignore functionality
// across dump, plan, and apply commands by testing the complete workflow with
// various database object types and ignore patterns including wildcards and negation.

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/apply"
	"github.com/pgplex/pgschema/cmd/dump"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/testutil"
	"github.com/spf13/cobra"
)

// Note: This file shares the TestMain and sharedEmbeddedPG from migrate_integration_test.go
// since they're in the same package (cmd)

func TestIgnoreIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create containerInfo struct to match old API for minimal changes
	containerInfo := &struct {
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

	// Create the test schema with various object types
	createTestSchema(t, containerInfo.Conn)

	// Save current working directory and restore it at the end
	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer func() {
		os.Chdir(originalWd)
	}()

	// Create a temporary directory for our tests
	tmpDir := t.TempDir()
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Run sub-tests in isolated environments
	t.Run("dump", func(t *testing.T) {
		testIgnoreDump(t, containerInfo)
	})

	t.Run("plan", func(t *testing.T) {
		testIgnorePlan(t, containerInfo)
	})

	t.Run("dependencies_on_ignored_tables", func(t *testing.T) {
		testDependenciesOnIgnoredTables(t, containerInfo)
	})

	t.Run("apply", func(t *testing.T) {
		// Create a fresh container for apply test to avoid fingerprint conflicts
		applyEmbeddedPG := testutil.SetupPostgres(t)
		defer applyEmbeddedPG.Stop()
		applyConn, applyHost, applyPort, applyDbname, applyUser, applyPassword := testutil.ConnectToPostgres(t, applyEmbeddedPG)
		defer applyConn.Close()

		// Create applyContainerInfo struct to match old API
		applyContainerInfo := &struct {
			Conn     *sql.DB
			Host     string
			Port     int
			DBName   string
			User     string
			Password string
		}{
			Conn:     applyConn,
			Host:     applyHost,
			Port:     applyPort,
			DBName:   applyDbname,
			User:     applyUser,
			Password: applyPassword,
		}

		// Create the test schema in the fresh container
		createTestSchema(t, applyContainerInfo.Conn)

		testIgnoreApply(t, applyContainerInfo)
	})
}

// createTestSchema creates all test objects in the database
func createTestSchema(t *testing.T, conn *sql.DB) {
	testSQL := `
-- Create user status enum type
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');

-- Create test enum type (to be ignored)
CREATE TYPE type_test_enum AS ENUM ('test1', 'test2');

-- Create regular tables
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    status user_status DEFAULT 'active'
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total_amount DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    price DECIMAL(10,2) NOT NULL
);

-- Create temporary tables (to be ignored)
CREATE TABLE temp_backup (
    id SERIAL PRIMARY KEY,
    data TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE temp_cache (
    key TEXT PRIMARY KEY,
    value TEXT,
    expires_at TIMESTAMP
);

CREATE TABLE temp_session (
    session_id TEXT PRIMARY KEY,
    user_id INTEGER,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create test tables (to be ignored, except core)
CREATE TABLE test_data (
    id SERIAL PRIMARY KEY,
    test_value TEXT
);

CREATE TABLE test_results (
    id SERIAL PRIMARY KEY,
    result TEXT
);

-- Create test core table (NOT ignored due to negation pattern)
CREATE TABLE test_core_config (
    id SERIAL PRIMARY KEY,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL
);

-- Create regular sequences
CREATE SEQUENCE user_id_seq;

-- Create temp sequence (to be ignored)
CREATE SEQUENCE seq_temp_counter;

-- Create regular views
CREATE VIEW user_orders_view AS
SELECT u.name, u.email, o.total_amount, o.created_at
FROM users u
JOIN orders o ON u.id = o.user_id;

CREATE VIEW product_summary AS
SELECT COUNT(*) as total_products, AVG(price) as avg_price
FROM products;

-- Create debug views (to be ignored)
CREATE VIEW debug_performance AS
SELECT 'debug_data' as info;

CREATE VIEW debug_stats AS
SELECT 'debug_stats' as stats;

-- Create temp view (to be ignored)
CREATE VIEW orders_view_tmp AS
SELECT * FROM orders WHERE created_at > NOW() - INTERVAL '1 hour';

-- Create regular functions
CREATE OR REPLACE FUNCTION get_user_count() RETURNS INTEGER AS $$
BEGIN
    RETURN (SELECT COUNT(*) FROM users);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION calculate_total(p_user_id INTEGER) RETURNS DECIMAL AS $$
BEGIN
    RETURN (SELECT COALESCE(SUM(total_amount), 0) FROM orders WHERE user_id = p_user_id);
END;
$$ LANGUAGE plpgsql;

-- Create test functions (to be ignored)
CREATE OR REPLACE FUNCTION fn_test_helper() RETURNS TEXT AS $$
BEGIN
    RETURN 'test helper';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_debug_log(p_message TEXT) RETURNS VOID AS $$
BEGIN
    -- Debug function
    RETURN;
END;
$$ LANGUAGE plpgsql;

-- Create regular procedure
CREATE OR REPLACE PROCEDURE process_orders()
LANGUAGE plpgsql
AS $$
BEGIN
    -- Process orders logic
    UPDATE orders SET total_amount = total_amount * 1.1 WHERE total_amount > 100;
END;
$$;

-- Create temp procedure (to be ignored)
CREATE OR REPLACE PROCEDURE sp_temp_cleanup()
LANGUAGE plpgsql
AS $$
BEGIN
    DELETE FROM temp_cache WHERE expires_at < NOW();
END;
$$;

-- Create external table (to be ignored)
CREATE TABLE temp_external_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
`

	_, err := conn.Exec(testSQL)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}
}

// createIgnoreFile creates a .pgschemaignore file in the current directory
func createIgnoreFile(t *testing.T) func() {
	ignoreContent := `[tables]
patterns = ["temp_*", "test_*", "!test_core_*"]

[views]
patterns = ["debug_*", "*_view_tmp"]

[functions]
patterns = ["fn_test_*", "fn_debug_*"]

[procedures]
patterns = ["sp_temp_*"]

[types]
patterns = ["type_test_*"]

[sequences]
patterns = ["seq_temp_*"]
`

	err := os.WriteFile(".pgschemaignore", []byte(ignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .pgschemaignore file: %v", err)
	}

	// Return cleanup function
	return func() {
		os.Remove(".pgschemaignore")
	}
}

// testIgnoreDump tests the dump command with ignore functionality
func testIgnoreDump(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}) {
	// Create .pgschemaignore file
	cleanup := createIgnoreFile(t)
	defer cleanup()

	// Execute dump command
	output := executeIgnoreDumpCommand(t, containerInfo)

	// Verify output contains expected objects and excludes ignored ones
	verifyDumpOutput(t, output)
}

// testDependenciesOnIgnoredTables tests that dependencies (FK, triggers, views) on ignored tables are preserved
// This consolidated test covers:
// - Triggers on ignored tables (issue #56)
// - Foreign keys to ignored tables (issue #167)
// - Views referencing ignored tables
// Tests both single-file and multi-file dump modes, plus plan command
func testDependenciesOnIgnoredTables(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}) {
	// Create additional test objects (reuse existing temp_external_users, users, user_status from createTestSchema)
	createSQL := `
-- External/ignored table for FK test (temp_* pattern)
CREATE TABLE temp_external_suppliers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    contact_email TEXT
);

-- Managed table with FK to ignored table
CREATE TABLE supplier_contracts (
    id SERIAL PRIMARY KEY,
    supplier_id INTEGER NOT NULL,
    contract_value DECIMAL(10,2) NOT NULL,
    CONSTRAINT fk_supplier FOREIGN KEY (supplier_id) REFERENCES temp_external_suppliers(id)
);

-- Trigger function for syncing from ignored table (reuses existing temp_external_users and users)
CREATE OR REPLACE FUNCTION sync_external_user_profile()
RETURNS trigger AS $$
BEGIN
    INSERT INTO users (name, email, status)
    VALUES ('External User', NEW.email, 'active');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger on ignored external table (reuses temp_external_users from createTestSchema)
CREATE TRIGGER on_external_user_created
    AFTER INSERT ON temp_external_users
    FOR EACH ROW
    EXECUTE FUNCTION sync_external_user_profile();

-- View that references ignored table
CREATE VIEW supplier_contract_summary AS
SELECT s.name, s.contact_email, c.contract_value
FROM temp_external_suppliers s
JOIN supplier_contracts c ON s.id = c.supplier_id;
`
	_, err := containerInfo.Conn.Exec(createSQL)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	// Clean up after test (don't drop shared objects from createTestSchema)
	defer func() {
		containerInfo.Conn.Exec("DROP VIEW IF EXISTS supplier_contract_summary CASCADE")
		containerInfo.Conn.Exec("DROP TRIGGER IF EXISTS on_external_user_created ON temp_external_users CASCADE")
		containerInfo.Conn.Exec("DROP FUNCTION IF EXISTS sync_external_user_profile() CASCADE")
		containerInfo.Conn.Exec("DROP TABLE IF EXISTS supplier_contracts CASCADE")
		containerInfo.Conn.Exec("DROP TABLE IF EXISTS temp_external_suppliers CASCADE")
	}()

	// Create .pgschemaignore file
	cleanup := createIgnoreFile(t)
	defer cleanup()

	// Test 1: Single-file dump
	t.Run("single_file_dump", func(t *testing.T) {
		output := executeIgnoreDumpCommand(t, containerInfo)

		// Verify ignored tables are NOT in dump
		if strings.Contains(output, "CREATE TABLE IF NOT EXISTS temp_external_suppliers") {
			t.Error("Dump should not include ignored table temp_external_suppliers")
		}
		if strings.Contains(output, "CREATE TABLE IF NOT EXISTS temp_external_users") {
			t.Error("Dump should not include ignored table temp_external_users")
		}

		// Verify FK constraint to ignored table IS preserved
		if !strings.Contains(output, "fk_supplier") {
			t.Error("Dump should include FK constraint fk_supplier")
		}
		if !strings.Contains(output, "temp_external_suppliers") {
			t.Error("FK constraint should reference temp_external_suppliers")
		}

		// Verify trigger on ignored table IS preserved
		if !strings.Contains(output, "on_external_user_created") {
			t.Error("Dump should include trigger on_external_user_created")
		}

		// Verify view referencing ignored table IS preserved
		if !strings.Contains(output, "supplier_contract_summary") {
			t.Error("Dump should include view supplier_contract_summary")
		}
	})

	// Test 2: Multi-file dump (issue #167 bug was here)
	t.Run("multi_file_dump", func(t *testing.T) {
		tmpDir := t.TempDir()
		outputFile := filepath.Join(tmpDir, "schema.sql")

		config := &dump.DumpConfig{
			Host:      containerInfo.Host,
			Port:      containerInfo.Port,
			DB:        containerInfo.DBName,
			User:      containerInfo.User,
			Password:  containerInfo.Password,
			Schema:    "public",
			MultiFile: true,
			File:      outputFile,
		}

		_, err := dump.ExecuteDump(config)
		if err != nil {
			t.Fatalf("Failed to execute multi-file dump: %v", err)
		}

		// Read supplier_contracts table file (should have FK)
		tablesDir := filepath.Join(tmpDir, "tables")
		contractsFile := filepath.Join(tablesDir, "supplier_contracts.sql")
		contractsContent, err := os.ReadFile(contractsFile)
		if err != nil {
			t.Fatalf("Failed to read supplier_contracts.sql: %v", err)
		}
		contractsOutput := string(contractsContent)

		// Verify FK constraint is in the table file
		if !strings.Contains(contractsOutput, "fk_supplier") {
			t.Error("Multi-file dump should include FK constraint fk_supplier in supplier_contracts.sql")
		}
		if !strings.Contains(contractsOutput, "temp_external_suppliers") {
			t.Error("FK constraint should reference temp_external_suppliers in multi-file dump")
		}

		// Verify view file exists and references ignored table
		viewsDir := filepath.Join(tmpDir, "views")
		viewFile := filepath.Join(viewsDir, "supplier_contract_summary.sql")
		viewContent, err := os.ReadFile(viewFile)
		if err != nil {
			t.Fatalf("Failed to read supplier_contract_summary.sql: %v", err)
		}
		if !strings.Contains(string(viewContent), "temp_external_suppliers") {
			t.Error("View should reference temp_external_suppliers in multi-file dump")
		}
	})

	// Test 3: Plan command
	t.Run("plan", func(t *testing.T) {
		// The dump and multi-file tests already verify that dependencies are preserved in output.
		// Plan test verifies that when given a desired state schema file with dependencies on
		// ignored tables, the plan doesn't try to DROP or CREATE those ignored tables.

		// Create schema file with modified version (add a column) to generate a plan
		schemaWithDeps := `
-- Reuse existing objects but with a modification to generate a diff
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    status user_status DEFAULT 'active'
);

-- External tables (ignored)
CREATE TABLE temp_external_suppliers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    contact_email TEXT
);

CREATE TABLE temp_external_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Modified table with FK to ignored table (add new column to generate diff)
CREATE TABLE supplier_contracts (
    id SERIAL PRIMARY KEY,
    supplier_id INTEGER NOT NULL,
    contract_value DECIMAL(10,2) NOT NULL,
    notes TEXT,  -- NEW COLUMN
    CONSTRAINT fk_supplier FOREIGN KEY (supplier_id) REFERENCES temp_external_suppliers(id)
);

-- Trigger function
CREATE OR REPLACE FUNCTION sync_external_user_profile()
RETURNS trigger AS $$
BEGIN
    INSERT INTO users (name, email, status)
    VALUES ('External User', NEW.email, 'active');
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger on ignored table
CREATE TRIGGER on_external_user_created
    AFTER INSERT ON temp_external_users
    FOR EACH ROW
    EXECUTE FUNCTION sync_external_user_profile();

-- View referencing ignored table
CREATE VIEW supplier_contract_summary AS
SELECT s.name, s.contact_email, c.contract_value
FROM temp_external_suppliers s
JOIN supplier_contracts c ON s.id = c.supplier_id;
`
		schemaFile := "schema_with_deps.sql"
		err := os.WriteFile(schemaFile, []byte(schemaWithDeps), 0644)
		if err != nil {
			t.Fatalf("Failed to create schema file: %v", err)
		}
		defer os.Remove(schemaFile)

		output := executeIgnorePlanCommand(t, containerInfo, schemaFile)

		// Verify ignored tables are NOT in plan (no CREATE/DROP for them)
		if strings.Contains(output, "CREATE TABLE IF NOT EXISTS temp_external_suppliers") ||
			strings.Contains(output, "DROP TABLE IF EXISTS temp_external_suppliers") {
			t.Error("Plan should not create or drop ignored table temp_external_suppliers")
		}
		if strings.Contains(output, "CREATE TABLE IF NOT EXISTS temp_external_users") ||
			strings.Contains(output, "DROP TABLE IF EXISTS temp_external_users") {
			t.Error("Plan should not create or drop ignored table temp_external_users")
		}

		// Verify the plan includes operations on managed objects that reference ignored tables
		// (The FK, trigger, and view should all be present in the desired state and not cause errors)
		if !strings.Contains(output, "supplier_contracts") {
			t.Error("Plan should include operations on supplier_contracts (table with FK to ignored table)")
		}
	})
}

// testIgnorePlan tests the plan command with ignore functionality
func testIgnorePlan(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}) {
	// Create .pgschemaignore file
	cleanup := createIgnoreFile(t)
	defer cleanup()

	// Create a modified schema file with changes to both regular and ignored objects
	modifiedSchema := `
-- User status enum type (needed for users table)
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');

-- Modified regular table (should appear in plan)
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    status user_status DEFAULT 'active',
    phone TEXT -- NEW COLUMN
);

-- Modified ignored table (should NOT appear in plan)
CREATE TABLE temp_backup (
    id SERIAL PRIMARY KEY,
    data TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    backup_type TEXT -- NEW COLUMN - should be ignored
);

-- Keep test_core_config (should appear due to negation)
CREATE TABLE test_core_config (
    id SERIAL PRIMARY KEY,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT NOW() -- NEW COLUMN
);
`

	schemaFile := "modified_schema.sql"
	err := os.WriteFile(schemaFile, []byte(modifiedSchema), 0644)
	if err != nil {
		t.Fatalf("Failed to create modified schema file: %v", err)
	}
	defer os.Remove(schemaFile)

	// Execute plan command
	output := executeIgnorePlanCommand(t, containerInfo, schemaFile)

	// Verify plan output excludes ignored objects
	verifyPlanOutput(t, output)
}

// testIgnoreApply tests the apply command with ignore functionality
// This test verifies that ignored objects are excluded from fingerprint calculation
func testIgnoreApply(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}) {
	// Create .pgschemaignore file
	cleanup := createIgnoreFile(t)
	defer cleanup()

	// Verify that ignored objects exist before apply
	verifyIgnoredObjectsExist(t, containerInfo.Conn, "before apply")

	// Create a schema file with ONLY regular (non-ignored) objects
	// This schema does NOT include ignored objects like sp_temp_cleanup, temp_*, fn_test_*, etc.
	regularObjectsSchema := `
-- Regular enum type (not ignored)
CREATE TYPE user_status AS ENUM ('active', 'inactive', 'suspended');

-- Regular tables (not ignored)
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    status user_status DEFAULT 'active'
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total_amount DECIMAL(10,2) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    price DECIMAL(10,2) NOT NULL
);

-- Keep test_core_config (not ignored due to negation pattern !test_core_*)
CREATE TABLE test_core_config (
    id SERIAL PRIMARY KEY,
    config_key TEXT NOT NULL,
    config_value TEXT NOT NULL
);

-- Regular sequence (not ignored)
CREATE SEQUENCE IF NOT EXISTS user_id_seq;

-- Regular views (not ignored)
CREATE VIEW user_orders_view AS
SELECT u.name, u.email, o.total_amount, o.created_at
FROM users u
JOIN orders o ON u.id = o.user_id;

CREATE VIEW product_summary AS
SELECT COUNT(*) as total_products, AVG(price) as avg_price
FROM products;

-- Regular functions (not ignored)
CREATE OR REPLACE FUNCTION get_user_count() RETURNS INTEGER AS $$
BEGIN
    RETURN (SELECT COUNT(*) FROM users);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION calculate_total(p_user_id INTEGER) RETURNS DECIMAL AS $$
BEGIN
    RETURN (SELECT COALESCE(SUM(total_amount), 0) FROM orders WHERE user_id = p_user_id);
END;
$$ LANGUAGE plpgsql;

-- Regular procedure (not ignored)
CREATE OR REPLACE PROCEDURE process_orders()
LANGUAGE plpgsql
AS $$
BEGIN
    -- Process orders logic
    UPDATE orders SET total_amount = total_amount * 1.1 WHERE total_amount > 100;
END;
$$;
`

	schemaFile := "regular_objects_schema.sql"
	err := os.WriteFile(schemaFile, []byte(regularObjectsSchema), 0644)
	if err != nil {
		t.Fatalf("Failed to create schema file: %v", err)
	}
	defer os.Remove(schemaFile)

	// Execute apply command - should succeed because ignored objects are excluded from fingerprint
	err = executeIgnoreApplyCommandWithError(containerInfo, schemaFile)
	if err != nil {
		t.Fatalf("Apply command should succeed when ignored objects are excluded from fingerprint, but got error: %v", err)
	}

	// Verify that ignored objects still exist after apply (they should remain untouched)
	verifyIgnoredObjectsExist(t, containerInfo.Conn, "after apply")

	// Verify that the ignored procedure sp_temp_cleanup still exists
	verifyIgnoredProcedureExists(t, containerInfo.Conn, "after apply")
}

// executeIgnoreDumpCommand runs the dump command and returns the output
func executeIgnoreDumpCommand(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}) string {
	// Create a new root command with dump as subcommand
	rootCmd := &cobra.Command{
		Use: "pgschema",
	}
	rootCmd.AddCommand(dump.DumpCmd)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	var output string
	done := make(chan bool)
	go func() {
		defer close(done)
		buf := make([]byte, 1024*1024) // 1MB buffer
		n, _ := r.Read(buf)
		output = string(buf[:n])
	}()

	// Set command arguments
	args := []string{
		"dump",
		"--host", containerInfo.Host,
		"--port", fmt.Sprintf("%d", containerInfo.Port),
		"--db", containerInfo.DBName,
		"--user", containerInfo.User,
		"--password", containerInfo.Password,
		"--schema", "public",
	}
	rootCmd.SetArgs(args)

	// Execute the command
	err := rootCmd.Execute()
	w.Close()
	os.Stdout = oldStdout
	<-done

	if err != nil {
		t.Fatalf("Failed to execute dump command: %v", err)
	}

	return output
}

// executeIgnorePlanCommand runs the plan command and returns the output
func executeIgnorePlanCommand(t *testing.T, containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, schemaFile string) string {
	// Create plan configuration with shared embedded postgres for performance
	config := &planCmd.PlanConfig{
		Host:            containerInfo.Host,
		Port:            containerInfo.Port,
		DB:              containerInfo.DBName,
		User:            containerInfo.User,
		Password:        containerInfo.Password,
		Schema:          "public",
		File:            schemaFile,
		ApplicationName: "pgschema",
	}

	// Generate the plan (reuse shared embedded postgres from migrate_integration_test.go)
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to execute plan command: %v", err)
	}

	// Return human-readable output (no color, like stdout)
	return migrationPlan.HumanColored(false)
}

// executeIgnoreApplyCommandWithError runs the apply command and returns any error
func executeIgnoreApplyCommandWithError(containerInfo *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, schemaFile string) error {
	rootCmd := &cobra.Command{
		Use: "pgschema",
	}
	rootCmd.AddCommand(apply.ApplyCmd)

	args := []string{
		"apply",
		"--host", containerInfo.Host,
		"--port", fmt.Sprintf("%d", containerInfo.Port),
		"--db", containerInfo.DBName,
		"--user", containerInfo.User,
		"--password", containerInfo.Password,
		"--schema", "public",
		"--file", schemaFile,
		"--auto-approve",
	}
	rootCmd.SetArgs(args)

	return rootCmd.Execute()
}

// verifyIgnoredObjectsExist checks that ignored objects still exist in the database
func verifyIgnoredObjectsExist(t *testing.T, conn *sql.DB, phase string) {
	// Check that temp_backup table still exists (should be ignored)
	var tempTableExists bool
	err := conn.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'temp_backup'
			AND table_schema = 'public'
		)
	`).Scan(&tempTableExists)

	if err != nil {
		t.Fatalf("Failed to check temp_backup table existence %s: %v", phase, err)
	}

	if !tempTableExists {
		t.Errorf("temp_backup table should exist %s (ignored tables should remain unchanged)", phase)
	}

	// Check that test_data table still exists (should be ignored)
	var testTableExists bool
	err = conn.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'test_data'
			AND table_schema = 'public'
		)
	`).Scan(&testTableExists)

	if err != nil {
		t.Fatalf("Failed to check test_data table existence %s: %v", phase, err)
	}

	if !testTableExists {
		t.Errorf("test_data table should exist %s (ignored tables should remain unchanged)", phase)
	}
}

// verifyIgnoredProcedureExists checks that the ignored procedure sp_temp_cleanup still exists
func verifyIgnoredProcedureExists(t *testing.T, conn *sql.DB, phase string) {
	var procedureExists bool
	err := conn.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.routines
			WHERE routine_name = 'sp_temp_cleanup'
			AND routine_schema = 'public'
			AND routine_type = 'PROCEDURE'
		)
	`).Scan(&procedureExists)

	if err != nil {
		t.Fatalf("Failed to check sp_temp_cleanup procedure existence %s: %v", phase, err)
	}

	if !procedureExists {
		t.Errorf("sp_temp_cleanup procedure should exist %s (ignored procedures should remain unchanged)", phase)
	}
}

// verifyDumpOutput checks that dump output contains expected objects and excludes ignored ones
func verifyDumpOutput(t *testing.T, output string) {
	// Objects that should be present (not ignored)
	expectedPresent := []string{
		"CREATE TABLE IF NOT EXISTS users",
		"CREATE TABLE IF NOT EXISTS orders",
		"CREATE TABLE IF NOT EXISTS products",
		"CREATE TABLE IF NOT EXISTS test_core_config", // Not ignored due to negation
		"CREATE OR REPLACE VIEW user_orders_view",
		"CREATE OR REPLACE VIEW product_summary",
		"CREATE OR REPLACE FUNCTION get_user_count",
		"CREATE OR REPLACE FUNCTION calculate_total",
		"CREATE OR REPLACE PROCEDURE process_orders",
		"CREATE TYPE user_status",
		"CREATE SEQUENCE IF NOT EXISTS user_id_seq",
	}

	// Objects that should be absent (ignored)
	expectedAbsent := []string{
		"temp_backup",
		"temp_cache",
		"temp_session",
		"test_data",
		"test_results",
		"debug_performance",
		"debug_stats",
		"orders_view_tmp",
		"fn_test_helper",
		"fn_debug_log",
		"sp_temp_cleanup",
		"type_test_enum",
		"seq_temp_counter",
	}

	// Check for expected present objects
	for _, expected := range expectedPresent {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected object not found in dump output: %s", expected)
		}
	}

	// Check for expected absent objects
	for _, unexpected := range expectedAbsent {
		if strings.Contains(output, unexpected) {
			t.Errorf("Ignored object found in dump output (should be excluded): %s", unexpected)
		}
	}
}

func TestIgnorePrivileges(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	containerInfo := &struct {
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

	// Create schema with roles and privileges
	setupSQL := `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total_amount DECIMAL(10,2) NOT NULL
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_reader') THEN
        CREATE ROLE app_reader;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'deploy_bot') THEN
        CREATE ROLE deploy_bot;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'admin_role') THEN
        CREATE ROLE admin_role;
    END IF;
END $$;

-- Privileges to keep
GRANT SELECT ON users TO app_reader;
GRANT SELECT ON orders TO app_reader;

-- Privileges to ignore (deploy_bot)
GRANT ALL ON users TO deploy_bot;
GRANT ALL ON orders TO deploy_bot;

-- Privileges to ignore (admin_role)
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO admin_role;

-- Default privileges to keep
ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO app_reader;

-- Default privileges to ignore (deploy_bot)
ALTER DEFAULT PRIVILEGES GRANT ALL ON TABLES TO deploy_bot;
`
	_, err := conn.Exec(setupSQL)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(originalWd); err != nil {
			t.Fatalf("Failed to restore working directory: %v", err)
		}
	}()

	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create .pgschemaignore with privileges section
	ignoreContent := `[privileges]
patterns = ["deploy_bot", "admin_*"]

[default_privileges]
patterns = ["deploy_bot"]
`
	err = os.WriteFile(".pgschemaignore", []byte(ignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .pgschemaignore: %v", err)
	}

	t.Run("dump", func(t *testing.T) {
		output := executeIgnoreDumpCommand(t, containerInfo)

		// Privileges for app_reader should be present
		if !strings.Contains(output, "app_reader") {
			t.Error("Dump should include privileges for app_reader")
		}

		// Privileges for deploy_bot should be ignored
		if strings.Contains(output, "deploy_bot") {
			t.Error("Dump should not include privileges for deploy_bot (ignored)")
		}

		// Privileges for admin_role should be ignored (matches admin_*)
		if strings.Contains(output, "admin_role") {
			t.Error("Dump should not include privileges for admin_role (ignored by admin_* pattern)")
		}
	})

	t.Run("plan", func(t *testing.T) {
		// Create schema file that adds new privileges
		schemaSQL := `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT NOT NULL
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total_amount DECIMAL(10,2) NOT NULL
);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_reader') THEN
        CREATE ROLE app_reader;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'deploy_bot') THEN
        CREATE ROLE deploy_bot;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'admin_role') THEN
        CREATE ROLE admin_role;
    END IF;
END $$;

-- Keep these privileges
GRANT SELECT ON users TO app_reader;
GRANT SELECT ON orders TO app_reader;

-- These should be ignored in plan
GRANT ALL ON users TO deploy_bot;
GRANT ALL ON orders TO deploy_bot;
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO admin_role;

-- Default privileges
ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO app_reader;
ALTER DEFAULT PRIVILEGES GRANT ALL ON TABLES TO deploy_bot;
`
		schemaFile := "schema_privs.sql"
		err := os.WriteFile(schemaFile, []byte(schemaSQL), 0644)
		if err != nil {
			t.Fatalf("Failed to create schema file: %v", err)
		}
		defer os.Remove(schemaFile)

		output := executeIgnorePlanCommand(t, containerInfo, schemaFile)

		// Plan should not contain any changes for ignored roles
		if strings.Contains(output, "deploy_bot") {
			t.Error("Plan should not include changes for deploy_bot (ignored)")
		}
		if strings.Contains(output, "admin_role") {
			t.Error("Plan should not include changes for admin_role (ignored)")
		}
	})

	// Clean up cluster-level objects (roles, default privileges) from sharedEmbeddedPG.
	// The plan subtest applies SQL with CREATE ROLE and ALTER DEFAULT PRIVILEGES to the
	// shared embedded PG instance. These are cluster-level objects that persist after the
	// temp schema is dropped, contaminating subsequent tests (e.g., TestPlanAndApply).
	cleanupSharedEmbeddedPG(t)
}

// cleanupSharedEmbeddedPG removes cluster-level objects (roles, default privileges)
// that were created in sharedEmbeddedPG by privilege tests.
func cleanupSharedEmbeddedPG(t *testing.T) {
	t.Helper()

	sharedConn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedEmbeddedPG)
	defer sharedConn.Close()

	// Must clean up in order: revoke default privileges, revoke object privileges, then drop roles.
	// Each statement runs independently since some roles may not exist.
	cleanupStatements := []string{
		"ALTER DEFAULT PRIVILEGES REVOKE ALL ON TABLES FROM app_reader",
		"ALTER DEFAULT PRIVILEGES REVOKE ALL ON TABLES FROM deploy_bot",
		"REASSIGN OWNED BY app_reader TO testuser",
		"DROP OWNED BY app_reader",
		"REASSIGN OWNED BY deploy_bot TO testuser",
		"DROP OWNED BY deploy_bot",
		"REASSIGN OWNED BY admin_role TO testuser",
		"DROP OWNED BY admin_role",
		"DROP ROLE IF EXISTS app_reader",
		"DROP ROLE IF EXISTS deploy_bot",
		"DROP ROLE IF EXISTS admin_role",
	}
	for _, stmt := range cleanupStatements {
		sharedConn.Exec(stmt) // Ignore errors; some roles may not exist
	}
}

// verifyPlanOutput checks that plan output excludes ignored objects
func verifyPlanOutput(t *testing.T, output string) {
	// Changes that should appear in plan (regular objects)
	expectedInPlan := []string{
		"users",            // Should show column addition
		"test_core_config", // Not ignored due to negation
	}

	// Changes that should NOT appear in plan (ignored objects)
	expectedNotInPlan := []string{
		"temp_backup", // Should be ignored
	}

	// Check that regular objects appear in plan
	for _, expected := range expectedInPlan {
		if !strings.Contains(output, expected) {
			t.Errorf("Expected object not found in plan output: %s", expected)
		}
	}

	// Check that ignored objects don't appear in plan
	for _, unexpected := range expectedNotInPlan {
		if strings.Contains(output, unexpected) {
			t.Errorf("Ignored object found in plan output (should be excluded): %s", unexpected)
		}
	}
}
