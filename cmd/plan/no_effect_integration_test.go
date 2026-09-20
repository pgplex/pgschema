package plan

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// TestPlan_WarnsAboutNoEffectStatements verifies that statements pgschema never
// plans are reported instead of silently yielding an empty plan: object
// OWNER TO (issue #602) and ALTER DEFAULT PRIVILEGES without IN SCHEMA
// (issue #603).
//
// The statements name a role that exists nowhere. They are dropped before role
// validation and before the plan database, so neither rejects them.
func TestPlan_WarnsAboutNoEffectStatements(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	ctx := context.Background()

	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, targetDB)
	defer conn.Close()

	_, err := conn.ExecContext(ctx, `CREATE TABLE item (id integer PRIMARY KEY, value integer NOT NULL);`)
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "schema.sql")
	require.NoError(t, os.WriteFile(file, []byte(`
		CREATE TABLE item (id integer PRIMARY KEY, value integer NOT NULL);
		ALTER TABLE item OWNER TO absent_role;
		ALTER DEFAULT PRIVILEGES FOR ROLE absent_role GRANT SELECT ON TABLES TO absent_role;
	`), 0644))

	var warnings bytes.Buffer
	warningWriter = &warnings
	defer func() { warningWriter = os.Stderr }()

	config := &PlanConfig{
		Host: host, Port: port, DB: dbname, User: user, Password: password,
		Schema: "public", File: file, ApplicationName: "pgschema-test",
	}
	provider, err := CreateDesiredStateProvider(config)
	require.NoError(t, err)
	defer provider.Stop()

	p, err := GeneratePlan(config, provider)
	require.NoError(t, err)
	require.False(t, p.HasAnyChanges(), "plan should be empty, got:\n%s", p.HumanColored(false))

	out := warnings.String()
	require.Contains(t, out, "Warning: statement has no effect: ALTER TABLE item OWNER TO absent_role")
	require.Contains(t, out, "unsupported#object-ownership")
	require.Contains(t, out, "Warning: statement has no effect: ALTER DEFAULT PRIVILEGES FOR ROLE absent_role GRANT SELECT ON TABLES TO absent_role")
	require.Contains(t, out, "unsupported#global-default-privileges")
}
