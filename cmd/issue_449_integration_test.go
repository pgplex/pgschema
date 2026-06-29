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
// Three distinct drift sources are covered:
//  1. CHECK constraints casting to a type in the managed (non-public) schema: the current
//     state rendered the cast schema-qualified while the desired state stripped the
//     qualifier, producing a perpetual DROP/ADD/VALIDATE cycle (also issue #445).
//  2. RLS policy expressions on a table whose name equals the managed schema name: the
//     same-schema qualifier stripping mistook table-qualified column references
//     (profiles.id) for schema qualifiers and removed them from the current state only.
//  3. Trigger WHEN expressions casting to an enum in the managed (non-public) schema:
//     pg_get_triggerdef rendered the cast schema-qualified (the schema is not on the
//     default search_path) while the desired state was unqualified, so a repeat plan kept
//     recreating the trigger (issue #481).
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

	t.Run("trigger_when_same_schema_enum_cast", func(t *testing.T) {
		replan := applyThenReplan(t, "content", `
			CREATE TYPE media_provider AS ENUM ('BUNNY_STREAM', 'OTHER');
			CREATE TYPE media_type AS ENUM ('VIDEO', 'IMAGE');

			CREATE TABLE media_items (
				id uuid PRIMARY KEY,
				post_id uuid,
				provider media_provider NOT NULL,
				type media_type NOT NULL,
				provider_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
				storage_info jsonb NOT NULL DEFAULT '{}'::jsonb
			);

			CREATE FUNCTION update_post_media_flags()
			RETURNS trigger
			LANGUAGE plpgsql
			AS $$
			BEGIN
				RETURN COALESCE(NEW, OLD);
			END;
			$$;

			CREATE TRIGGER update_post_media_flags_on_bunny_metadata_update
				AFTER UPDATE OF provider_payload, storage_info ON media_items
				FOR EACH ROW
				WHEN (
					NEW.post_id IS NOT NULL
					AND NEW.provider = 'BUNNY_STREAM'::media_provider
					AND NEW.type = 'VIDEO'::media_type
					AND (
						OLD.provider_payload IS DISTINCT FROM NEW.provider_payload
						OR OLD.storage_info IS DISTINCT FROM NEW.storage_info
					)
				)
				EXECUTE FUNCTION update_post_media_flags();
		`)
		if replan != "" {
			t.Errorf("Expected no changes on repeat plan after apply, but got:\n%s", replan)
		}
	})
}
