package dump

// Permission Integration Tests
// These tests verify that pgschema handles permission errors properly when
// encountering database objects owned by inaccessible roles.
//
// This reproduces and verifies the fix for issue #32 where pgschema was outputting
// "%!s(<nil>)" instead of proper error handling for procedures/functions
// owned by roles the user doesn't have access to.

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/testutil"
)

// TestDumpCommand_PermissionSuite verifies that pgschema handles permission errors properly
// when encountering database objects owned by inaccessible roles.
func TestDumpCommand_PermissionSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start single PostgreSQL container for all permission tests
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

	// Run each permission test with its own isolated database
	t.Run("ProcedureAndFunctionSourceAccess", func(t *testing.T) {
		testProcedureAndFunctionSourceAccess(t, ctx, container, "testdb_source")
	})

	t.Run("IgnoredObjects", func(t *testing.T) {
		testIgnoredObjects(t, ctx, container, "testdb_ignore")
	})
}

// setupTestDatabase creates a new database with permission test roles
func setupTestDatabase(ctx context.Context, t *testing.T, container *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, dbName string) *sql.DB {
	// Create the database
	_, err := container.Conn.ExecContext(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		t.Fatalf("Failed to create database %s: %v", dbName, err)
	}

	// Create unique role names for this test
	restrictedRole := fmt.Sprintf("restricted_owner_%s", dbName)
	regularUser := fmt.Sprintf("regular_user_%s", dbName)

	// Create roles in the main postgres connection
	_, err = container.Conn.ExecContext(ctx, fmt.Sprintf(`
		-- Create a restricted role that regular users can't access
		CREATE ROLE %s;

		-- Create a regular user without access to restricted_owner
		CREATE USER %s WITH PASSWORD 'userpass';
		GRANT CONNECT ON DATABASE %s TO %s;
	`, restrictedRole, regularUser, dbName, regularUser))
	if err != nil {
		t.Fatalf("Failed to setup permission test roles for %s: %v", dbName, err)
	}

	// Connect to the new database
	config := &util.ConnectionConfig{
		Host:            container.Host,
		Port:            container.Port,
		Database:        dbName,
		User:            container.User,
		Password:        container.Password,
		SSLMode:         "prefer",
		ApplicationName: "pgschema",
	}

	dbConn, err := util.Connect(config)
	if err != nil {
		t.Fatalf("Failed to connect to database %s: %v", dbName, err)
	}

	// Grant schema permissions within the database
	_, err = dbConn.ExecContext(ctx, fmt.Sprintf(`
		GRANT USAGE ON SCHEMA public TO %s;
	`, regularUser))
	if err != nil {
		t.Fatalf("Failed to setup schema permissions for %s: %v", dbName, err)
	}

	return dbConn
}

// getRoleNames returns the unique role names for a given database
func getRoleNames(dbName string) (restrictedRole string, regularUser string) {
	return fmt.Sprintf("restricted_owner_%s", dbName), fmt.Sprintf("regular_user_%s", dbName)
}

// testIgnoredObjects tests procedures ignored via pgschema.toml
func testIgnoredObjects(t *testing.T, ctx context.Context, container *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, dbName string) {
	// This test verifies that when procedures/functions are explicitly ignored
	// via pgschema.toml, permission issues should not cause the dump to fail

	// Setup isolated database
	dbConn := setupTestDatabase(ctx, t, container, dbName)
	defer dbConn.Close()

	// Get role names for this database
	restrictedRole, regularUser := getRoleNames(dbName)

	// Create both ignored procedures/functions with permission issues
	// and non-ignored ones that should still cause errors
	_, err := dbConn.ExecContext(ctx, fmt.Sprintf(`
		-- Create ignored procedure owned by restricted role
		CREATE OR REPLACE PROCEDURE public.skip_proc_restricted(param_text TEXT)
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE NOTICE 'This procedure is ignored and restricted: %%', param_text;
		END;
		$$;

		ALTER PROCEDURE skip_proc_restricted(TEXT) OWNER TO %s;

		-- Create ignored function owned by restricted role
		CREATE OR REPLACE FUNCTION public.skip_func_restricted(input_text TEXT)
		RETURNS TEXT
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RETURN 'Ignored function result: ' || input_text;
		END;
		$$;

		ALTER FUNCTION skip_func_restricted(TEXT) OWNER TO %s;

		-- Create non-ignored procedure owned by restricted role (should still cause error)
		CREATE OR REPLACE PROCEDURE public.check_proc_restricted(param_int INTEGER)
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE NOTICE 'This procedure is not ignored and restricted: %%', param_int;
		END;
		$$;

		ALTER PROCEDURE check_proc_restricted(INTEGER) OWNER TO %s;
	`, restrictedRole, restrictedRole, restrictedRole))
	if err != nil {
		t.Fatalf("Failed to setup ignore permission test: %v", err)
	}

	// Create pgschema.toml file that ignores the specific procedures/functions
	ignoreContent := `[ignore]
procedures = ["skip_proc_restricted"]

functions = ["skip_func_restricted"]
`

	err = os.WriteFile("pgschema.toml", []byte(ignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create pgschema.toml file: %v", err)
	}

	// Cleanup function to remove pgschema.toml file
	defer func() {
		os.Remove("pgschema.toml")
	}()

	// Try to dump schema as regular_user
	// Should fail due to check_proc_restricted, but ignored objects should not cause issues
	output, err := executeDumpCommandAsUser(
		container.Host,
		container.Port,
		dbName,
		regularUser,
		"userpass",
		"public",
	)

	// Should not contain formatting errors
	if strings.Contains(output, "%!s(<nil>)") {
		t.Errorf("Found formatting error '%s' in dump output with ignored objects", "%!s(<nil>)")
	}

	// The dump should now succeed since all procedures are accessible via pg_get_functiondef
	if err != nil {
		t.Errorf("Dump should succeed since all procedures are readable via pg_get_functiondef, got error: %v", err)
	} else {
		t.Logf("Success: dump succeeded with ignored/non-ignored procedures")
		// Verify the non-ignored procedure is in output
		if !strings.Contains(output, "check_proc_restricted") {
			t.Errorf("Expected check_proc_restricted in output")
		}
		// Verify the ignored procedures are NOT in output (due to ignore rules)
		if strings.Contains(output, "skip_proc_restricted") {
			t.Errorf("skip_proc_restricted should be ignored and not appear in output")
		}
		if strings.Contains(output, "skip_func_restricted") {
			t.Errorf("skip_func_restricted should be ignored and not appear in output")
		}
	}

	// Additional test: Create pgschema.toml that ignores ALL restricted objects
	ignoreAllContent := `[ignore]
procedures = ["*_restricted"]

functions = ["*_restricted"]
`

	err = os.WriteFile("pgschema.toml", []byte(ignoreAllContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create comprehensive pgschema.toml file: %v", err)
	}

	// Now the dump should succeed and ALL restricted objects should be ignored
	output2, err2 := executeDumpCommandAsUser(
		container.Host,
		container.Port,
		dbName,
		regularUser,
		"userpass",
		"public",
	)

	if err2 != nil {
		t.Errorf("Dump should succeed when all restricted objects are ignored, got error: %v", err2)
	} else {
		t.Logf("Success: dump succeeded with all restricted objects ignored")
		// Verify that ALL restricted objects are ignored
		if strings.Contains(output2, "skip_proc_restricted") {
			t.Errorf("skip_proc_restricted should be ignored and not appear in output")
		}
		if strings.Contains(output2, "skip_func_restricted") {
			t.Errorf("skip_func_restricted should be ignored and not appear in output")
		}
		if strings.Contains(output2, "check_proc_restricted") {
			t.Errorf("check_proc_restricted should be ignored and not appear in output")
		}
	}

	// Should not contain formatting errors
	if strings.Contains(output2, "%!s(<nil>)") {
		t.Errorf("Found formatting error '%s' in comprehensive ignore dump output", "%!s(<nil>)")
	}
}

// testProcedureAndFunctionSourceAccess tests that procedure and function source code is readable
// via p.prosrc even when information_schema.routines.routine_definition is NULL
func testProcedureAndFunctionSourceAccess(t *testing.T, ctx context.Context, container *struct {
	Conn     *sql.DB
	Host     string
	Port     int
	DBName   string
	User     string
	Password string
}, dbName string) {
	// Setup isolated database
	dbConn := setupTestDatabase(ctx, t, container, dbName)
	defer dbConn.Close()

	// Get role names for this database
	restrictedRole, regularUser := getRoleNames(dbName)

	// Create both procedure and function owned by restricted role
	_, err := dbConn.ExecContext(ctx, fmt.Sprintf(`
		-- Create a procedure owned by the restricted role
		CREATE OR REPLACE PROCEDURE public.test_source_visibility_proc(param_name TEXT)
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE NOTICE 'Procedure source visibility test: %%', param_name;
		END;
		$$;

		-- Create a function owned by the restricted role
		CREATE OR REPLACE FUNCTION public.test_source_visibility_func(param_name TEXT)
		RETURNS TEXT
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RETURN 'Function source visibility test: ' || param_name;
		END;
		$$;

		-- Change ownership to the restricted role
		ALTER PROCEDURE public.test_source_visibility_proc(TEXT) OWNER TO %s;
		ALTER FUNCTION public.test_source_visibility_func(TEXT) OWNER TO %s;
	`, restrictedRole, restrictedRole))
	if err != nil {
		t.Fatalf("Failed to setup procedure and function for source visibility test: %v", err)
	}

	// Connect as the regular user to test visibility
	config := &util.ConnectionConfig{
		Host:            container.Host,
		Port:            container.Port,
		Database:        dbName,
		User:            regularUser,
		Password:        "userpass",
		SSLMode:         "prefer",
		ApplicationName: "pgschema",
	}

	regularUserConn, err := util.Connect(config)
	if err != nil {
		t.Fatalf("Failed to connect as regular user: %v", err)
	}
	defer regularUserConn.Close()

	// Test 1: Check that information_schema.routines.routine_definition can be NULL for procedures
	var procRoutineDefinition sql.NullString
	err = regularUserConn.QueryRowContext(ctx, `
		SELECT routine_definition
		FROM information_schema.routines
		WHERE routine_schema = 'public'
		  AND routine_name = 'test_source_visibility_proc'
		  AND routine_type = 'PROCEDURE'
	`).Scan(&procRoutineDefinition)

	if err != nil {
		t.Fatalf("Failed to query procedure routine_definition: %v", err)
	}

	// Test 2: Check that information_schema.routines.routine_definition can be NULL for functions
	var funcRoutineDefinition sql.NullString
	err = regularUserConn.QueryRowContext(ctx, `
		SELECT routine_definition
		FROM information_schema.routines
		WHERE routine_schema = 'public'
		  AND routine_name = 'test_source_visibility_func'
		  AND routine_type = 'FUNCTION'
	`).Scan(&funcRoutineDefinition)

	if err != nil {
		t.Fatalf("Failed to query function routine_definition: %v", err)
	}

	// Test 3: Check that pg_get_functiondef() works for procedures despite NULL routine_definition
	var procedureDef string
	err = regularUserConn.QueryRowContext(ctx, `
		SELECT pg_get_functiondef(p.oid)
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		WHERE n.nspname = 'public'
		  AND p.proname = 'test_source_visibility_proc'
	`).Scan(&procedureDef)

	if err != nil {
		t.Errorf("pg_get_functiondef should work for procedures but failed: %v", err)
	} else {
		// Verify we got the actual procedure definition
		if !strings.Contains(procedureDef, "CREATE OR REPLACE PROCEDURE") {
			t.Errorf("Expected complete CREATE PROCEDURE statement, got: %s", procedureDef)
		}
		if !strings.Contains(procedureDef, "Procedure source visibility test") {
			t.Errorf("Expected procedure body content, got: %s", procedureDef)
		}
	}

	// Test 4: Check that pg_get_functiondef() works for functions despite NULL routine_definition
	var functionDef string
	err = regularUserConn.QueryRowContext(ctx, `
		SELECT pg_get_functiondef(p.oid)
		FROM pg_proc p
		JOIN pg_namespace n ON p.pronamespace = n.oid
		WHERE n.nspname = 'public'
		  AND p.proname = 'test_source_visibility_func'
	`).Scan(&functionDef)

	if err != nil {
		t.Errorf("pg_get_functiondef should work for functions but failed: %v", err)
	} else {
		// Verify we got the actual function definition
		if !strings.Contains(functionDef, "CREATE OR REPLACE FUNCTION") {
			t.Errorf("Expected complete CREATE FUNCTION statement, got: %s", functionDef)
		}
		if !strings.Contains(functionDef, "Function source visibility test") {
			t.Errorf("Expected function body content, got: %s", functionDef)
		}
	}

	// Test 5: Try to dump schema as regular_user (should succeed with pg_get_functiondef)
	output, dumpErr := executeDumpCommandAsUser(
		container.Host,
		container.Port,
		dbName,
		regularUser,
		"userpass",
		"public",
	)

	// Check that output does not contain formatting errors
	if strings.Contains(output, "%!s(<nil>)") {
		t.Errorf("Found formatting error '%s' in dump output", "%!s(<nil>)")
		t.Logf("Full output:\n%s", output)
	}

	// The dump should succeed since we use p.prosrc instead of routine_definition
	if dumpErr != nil {
		t.Errorf("Dump should succeed with p.prosrc, but got error: %v", dumpErr)
		t.Logf("Full output:\n%s", output)
	} else {
		t.Logf("Success: dump succeeded using p.prosrc")
		// Verify the output contains both procedure and function definitions
		if !strings.Contains(output, "CREATE OR REPLACE PROCEDURE") {
			t.Errorf("Expected complete CREATE PROCEDURE statement in output")
		}
		if !strings.Contains(output, "CREATE OR REPLACE FUNCTION") {
			t.Errorf("Expected complete CREATE FUNCTION statement in output")
		}
		if !strings.Contains(output, "test_source_visibility_proc") {
			t.Errorf("Expected procedure name 'test_source_visibility_proc' in output")
		}
		if !strings.Contains(output, "test_source_visibility_func") {
			t.Errorf("Expected function name 'test_source_visibility_func' in output")
		}
		if !strings.Contains(output, "Procedure source visibility test") {
			t.Errorf("Expected procedure body content in output")
		}
		if !strings.Contains(output, "Function source visibility test") {
			t.Errorf("Expected function body content in output")
		}
	}

	// Summary: This test verifies that:
	// 1. information_schema.routines.routine_definition can be NULL for both procedures and functions
	// 2. p.prosrc works reliably and returns the source for both procedures and functions
	// 3. pgschema now uses p.prosrc and succeeds for both object types
}

// executeDumpCommandAsUser executes the pgschema dump command as a specific user
// This helper is used specifically for permission testing where we need to run
// the dump command with restricted database user credentials.
func executeDumpCommandAsUser(hostArg string, portArg int, database, userArg, password, schemaArg string) (string, error) {
	// Create dump configuration
	config := &DumpConfig{
		Host:      hostArg,
		Port:      portArg,
		DB:        database,
		User:      userArg,
		Password:  password,
		Schema:    schemaArg,
		MultiFile: false,
		File:      "",
	}

	// Execute dump
	return ExecuteDump(config)
}
