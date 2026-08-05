package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

// TestIssue526FunctionSetConfigRoundTrip verifies that all function-local SET
// clauses—not just search_path—are preserved through plan → apply → plan.
// (https://github.com/pgplex/pgschema/issues/526)
func TestIssue526FunctionSetConfigRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// applyThenReplan applies the desired state and returns the plan generated
	// immediately afterwards. An idempotent plan returns "".
	applyThenReplan := func(t *testing.T, schema, desiredSQL string) string {
		t.Helper()

		if _, err := conn.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
			t.Fatalf("Failed to create schema %s: %v", schema, err)
		}

		desiredStateFile := filepath.Join(t.TempDir(), "desired.sql")
		if err := os.WriteFile(desiredStateFile, []byte(desiredSQL), 0644); err != nil {
			t.Fatalf("Failed to write desired state file: %v", err)
		}

		if err := applySchemaChanges(host, port, dbname, user, password, schema, desiredStateFile); err != nil {
			t.Fatalf("Failed to apply desired state: %v", err)
		}

		replanOutput, err := generatePlanSQLFormatted(host, port, dbname, user, password, schema, desiredStateFile)
		if err != nil {
			t.Fatalf("Failed to generate repeat plan: %v", err)
		}
		return replanOutput
	}

	t.Run("function_with_multiple_set_clauses", func(t *testing.T) {
		replan := applyThenReplan(t, "app526", `
			CREATE FUNCTION proconfig_repro()
			RETURNS text
			LANGUAGE sql
			STABLE
			SET search_path = public
			SET TimeZone = 'UTC'
			AS $$ SELECT current_setting('TimeZone') $$;
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})

	t.Run("function_with_only_non_search_path_set", func(t *testing.T) {
		replan := applyThenReplan(t, "app526b", `
			CREATE FUNCTION timeout_func()
			RETURNS text
			LANGUAGE sql
			SET statement_timeout = '30s'
			AS $$ SELECT 'ok' $$;
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})

	t.Run("function_with_no_set_clauses_is_unaffected", func(t *testing.T) {
		replan := applyThenReplan(t, "app526c", `
			CREATE FUNCTION plain_func()
			RETURNS int
			LANGUAGE sql
			IMMUTABLE
			AS $$ SELECT 1 $$;
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})

	t.Run("function_with_search_path_only_still_works", func(t *testing.T) {
		replan := applyThenReplan(t, "app526d", `
			CREATE FUNCTION search_path_only()
			RETURNS text
			LANGUAGE sql
			SET search_path = pg_catalog
			AS $$ SELECT 'ok' $$;
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})
}
