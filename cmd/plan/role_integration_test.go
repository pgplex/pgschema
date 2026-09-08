package plan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedPlanDB_StubsReferencedRoles verifies that plan can consume a dump
// that grants privileges to roles, without the schema file creating those
// roles (issue #450).
//
// Roles are cluster-global and not managed by pgschema, so dump never emits
// CREATE ROLE. The throwaway plan database therefore has none of them, and
// every GRANT ... TO <role> used to fail with "role does not exist". plan now
// stubs each referenced role that exists on the target; a role the target
// does not have is reported up front instead of failing later at apply.
func TestEmbeddedPlanDB_StubsReferencedRoles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()

	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, targetDB)
	defer conn.Close()

	// Every role-bearing statement dump can emit: object GRANT, column GRANT,
	// policy TO, and ALTER DEFAULT PRIVILEGES with both a grantor and a grantee.
	_, err := conn.ExecContext(ctx, `
		CREATE ROLE app_workspace;
		CREATE ROLE reader;

		CREATE TABLE users (id integer PRIMARY KEY, email text NOT NULL);
		CREATE SEQUENCE ticket_seq;
		GRANT SELECT, INSERT ON users TO app_workspace;
		GRANT SELECT (id) ON users TO reader;
		GRANT USAGE ON SEQUENCE ticket_seq TO app_workspace;
		ALTER TABLE users ENABLE ROW LEVEL SECURITY;
		CREATE POLICY users_self ON users FOR SELECT TO app_workspace USING (true);
		ALTER DEFAULT PRIVILEGES FOR ROLE app_workspace IN SCHEMA public GRANT SELECT ON TABLES TO reader;
	`)
	require.NoError(t, err)

	plan := func(t *testing.T, desiredSQL string) error {
		t.Helper()
		dir := t.TempDir()
		file := filepath.Join(dir, "schema.sql")
		require.NoError(t, os.WriteFile(file, []byte(desiredSQL), 0644))

		config := &PlanConfig{
			Host: host, Port: port, DB: dbname, User: user, Password: password,
			Schema: "public", File: file, ApplicationName: "pgschema-test", ConfigDir: dir,
		}
		provider, err := CreateDesiredStateProvider(config)
		require.NoError(t, err)
		defer provider.Stop()

		p, err := GeneratePlan(config, provider)
		if err != nil {
			return err
		}
		require.False(t, p.HasAnyChanges(), "desired state matches target; plan should be empty, got:\n%s", p.HumanColored(false))
		return nil
	}

	t.Run("dump round-trips without CREATE ROLE", func(t *testing.T) {
		err := plan(t, `
			CREATE TABLE users (id integer PRIMARY KEY, email text NOT NULL);
			CREATE SEQUENCE ticket_seq;
			GRANT SELECT, INSERT ON users TO app_workspace;
			GRANT SELECT (id) ON users TO reader;
			GRANT USAGE ON SEQUENCE ticket_seq TO app_workspace;
			ALTER TABLE users ENABLE ROW LEVEL SECURITY;
			CREATE POLICY users_self ON users FOR SELECT TO app_workspace USING (true);
			ALTER DEFAULT PRIVILEGES FOR ROLE app_workspace IN SCHEMA public GRANT SELECT ON TABLES TO reader;
		`)
		require.NoError(t, err, "plan must succeed without CREATE ROLE in the desired state")
	})

	t.Run("role missing on target is reported at plan time", func(t *testing.T) {
		err := plan(t, `
			CREATE TABLE users (id integer PRIMARY KEY, email text NOT NULL);
			GRANT SELECT ON users TO ghost_role;
		`)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ghost_role")
		require.Contains(t, err.Error(), "does not exist on the target database")
	})
}
