package plan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedPlanDB_InstallsTargetExtensions verifies that the embedded plan
// database mirrors the extensions installed on the target database (issue #584).
//
// pgschema does not manage extensions (they are database-level objects), so a
// dump never emits CREATE EXTENSION. The desired-state SQL therefore references
// extension types that only exist if plan installs the extension into its
// throwaway database first — pinned to the same schema as on the target so the
// diff sees identical type qualification (issue #518).
//
// Three placements are covered, each a different mapping into the plan database:
//   - citext in public:          installed into public as-is
//   - hstore in a side schema:   the side schema is created, then the extension
//   - ltree in the managed schema: mapped to the temporary schema, since that is
//     where the managed schema's objects live during plan
func TestEmbeddedPlanDB_InstallsTargetExtensions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()

	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, targetDB)
	defer conn.Close()

	_, err := conn.ExecContext(ctx, `
		CREATE SCHEMA exts;
		CREATE SCHEMA app;
		CREATE EXTENSION citext SCHEMA public;
		CREATE EXTENSION hstore SCHEMA exts;
		CREATE EXTENSION ltree SCHEMA app;

		CREATE TABLE public.users (id integer PRIMARY KEY, email citext NOT NULL);
		CREATE TABLE public.products (id integer PRIMARY KEY, attrs exts.hstore);
		CREATE TABLE app.paths (id integer PRIMARY KEY, path app.ltree NOT NULL);
	`)
	require.NoError(t, err)

	run := func(t *testing.T, schema, desiredSQL string) {
		t.Helper()
		dir := t.TempDir()
		file := filepath.Join(dir, "schema.sql")
		require.NoError(t, os.WriteFile(file, []byte(desiredSQL), 0644))

		config := &PlanConfig{
			Host: host, Port: port, DB: dbname, User: user, Password: password,
			Schema: schema, File: file, ApplicationName: "pgschema-test", ConfigDir: dir,
		}
		provider, err := CreateDesiredStateProvider(config)
		require.NoError(t, err)
		defer provider.Stop()

		p, err := GeneratePlan(config, provider)
		require.NoError(t, err, "plan must succeed without CREATE EXTENSION in the desired state")
		require.False(t, p.HasAnyChanges(), "desired state matches target; plan should be empty, got:\n%s", p.HumanColored(false))
	}

	t.Run("extension in public", func(t *testing.T) {
		run(t, "public", `
			CREATE TABLE users (id integer PRIMARY KEY, email citext NOT NULL);
			CREATE TABLE products (id integer PRIMARY KEY, attrs exts.hstore);
		`)
	})

	t.Run("extension in managed schema", func(t *testing.T) {
		run(t, "app", `
			CREATE TABLE paths (id integer PRIMARY KEY, path ltree NOT NULL);
		`)
	})
}
