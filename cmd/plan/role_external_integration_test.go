package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// TestExternalDatabase_StubsReferencedRoles verifies that an external plan
// database gets stub roles for the schema's grantees, and that Stop drops only
// the roles pgschema created (issue #450).
func TestExternalDatabase_StubsReferencedRoles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()
	planDB := testutil.SetupPostgres(t)
	defer planDB.Stop()

	targetConn, targetHost, targetPort, targetDatabase, targetUser, targetPassword := testutil.ConnectToPostgres(t, targetDB)
	defer targetConn.Close()
	_, err := targetConn.Exec(`
		CREATE ROLE app_workspace;
		CREATE ROLE shared_role;
		CREATE TABLE users (id integer PRIMARY KEY);
		GRANT SELECT ON users TO app_workspace, shared_role;
	`)
	require.NoError(t, err)

	// shared_role already exists on the plan host; app_workspace does not.
	planConn, planHost, planPort, planDatabase, planUser, planPassword := testutil.ConnectToPostgres(t, planDB)
	defer planConn.Close()
	_, err = planConn.Exec("CREATE ROLE shared_role")
	require.NoError(t, err)

	roleExists := func(name string) bool {
		var exists bool
		require.NoError(t, planConn.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", name).Scan(&exists))
		return exists
	}

	dir := t.TempDir()
	file := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(file, []byte(`
		CREATE TABLE users (id integer PRIMARY KEY);
		GRANT SELECT ON users TO app_workspace, shared_role;
	`), 0644))

	config := &PlanConfig{
		Host: targetHost, Port: targetPort, DB: targetDatabase, User: targetUser, Password: targetPassword,
		Schema: "public", File: file, ApplicationName: "pgschema-test", ConfigDir: dir,
		PlanDBHost: planHost, PlanDBPort: planPort, PlanDBDatabase: planDatabase, PlanDBUser: planUser, PlanDBPassword: planPassword,
	}
	provider, err := CreateDesiredStateProvider(config)
	require.NoError(t, err)

	p, err := GeneratePlan(config, provider)
	require.NoError(t, err, "plan must succeed without CREATE ROLE in the desired state")
	require.False(t, p.HasAnyChanges(), "desired state matches target; plan should be empty, got:\n%s", p.HumanColored(false))
	require.True(t, roleExists("app_workspace"), "stub role should exist while the provider is alive")

	require.NoError(t, provider.Stop())
	require.False(t, roleExists("app_workspace"), "stub role must be dropped on Stop")
	require.True(t, roleExists("shared_role"), "pre-existing role must never be dropped")
}
