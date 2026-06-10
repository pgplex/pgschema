package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

// TestIssue449RepeatPlanIdempotency verifies that a plan generated immediately after a
// successful apply reports no changes (https://github.com/pgplex/pgschema/issues/449).
//
// Two distinct drift sources are covered:
//  1. CHECK constraints casting to a type in the managed (non-public) schema: the current
//     state rendered the cast schema-qualified while the desired state stripped the
//     qualifier, producing a perpetual DROP/ADD/VALIDATE cycle (also issue #445).
//  2. RLS policy expressions on a table whose name equals the managed schema name: the
//     same-schema qualifier stripping mistook table-qualified column references
//     (profiles.id) for schema qualifiers and removed them from the current state only.
func TestIssue449RepeatPlanIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// applyThenReplan applies the desired state to the given schema and returns the SQL
	// of a plan generated immediately afterwards. An idempotent plan returns "".
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

	t.Run("check_constraint_same_schema_type_cast", func(t *testing.T) {
		replan := applyThenReplan(t, "app449", `
			CREATE TYPE item_status AS ENUM ('active', 'disabled');

			CREATE TABLE items (
				id integer PRIMARY KEY,
				status item_status NOT NULL,
				CONSTRAINT items_status_check CHECK (status <> 'disabled'::item_status)
			);
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})

	t.Run("policy_schema_name_equals_table_name", func(t *testing.T) {
		replan := applyThenReplan(t, "profiles", `
			CREATE TABLE profiles (
				id uuid PRIMARY KEY,
				user_id uuid NOT NULL
			);

			CREATE TABLE profile_media (
				id uuid PRIMARY KEY,
				profile_id uuid NOT NULL REFERENCES profiles (id)
			);

			ALTER TABLE profile_media ENABLE ROW LEVEL SECURITY;

			CREATE POLICY "Users can read their own media" ON profile_media
				FOR SELECT
				USING (profile_id IN (SELECT id FROM profiles));
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})
}
