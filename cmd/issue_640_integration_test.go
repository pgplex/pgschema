package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

// TestIssue640PartitionedTablePrivileges verifies that GRANT/REVOKE/dump
// cover privileges on partitioned tables (relkind 'p'), not only ordinary
// tables. See https://github.com/pgplex/pgschema/issues/640.
func TestIssue640PartitionedTablePrivileges(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Roles are not managed by pgschema; create on the target before plan/apply.
	roleSQL := `
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reader') THEN
        CREATE ROLE reader;
    END IF;
END $$;
`
	if _, err := conn.ExecContext(ctx, roleSQL); err != nil {
		t.Fatalf("create role on target: %v", err)
	}

	tablesOnly := `
CREATE TABLE events (
    id int NOT NULL,
    at date NOT NULL
) PARTITION BY RANGE (at);

CREATE TABLE events_2026_01 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE plain (id int NOT NULL);
`
	withGrants := tablesOnly + `
GRANT SELECT ON events TO reader;
GRANT SELECT ON plain TO reader;
`

	schema := "public"
	tmp := t.TempDir()
	tablesFile := filepath.Join(tmp, "tables.sql")
	grantsFile := filepath.Join(tmp, "with_grants.sql")
	if err := os.WriteFile(tablesFile, []byte(tablesOnly), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(grantsFile, []byte(withGrants), 0644); err != nil {
		t.Fatal(err)
	}

	if err := applySchemaChanges(host, port, dbname, user, password, schema, tablesFile); err != nil {
		t.Fatalf("apply tables: %v", err)
	}

	planSQL, err := generatePlanSQLFormatted(host, port, dbname, user, password, schema, grantsFile)
	if err != nil {
		t.Fatalf("plan with grants: %v", err)
	}

	hasGrant := func(sql, obj string) bool {
		return strings.Contains(sql, "GRANT SELECT ON TABLE "+obj+" TO reader") ||
			strings.Contains(sql, "GRANT SELECT ON "+obj+" TO reader")
	}
	hasRevoke := func(sql, obj string) bool {
		return strings.Contains(sql, "REVOKE SELECT ON TABLE "+obj+" FROM reader") ||
			strings.Contains(sql, "REVOKE SELECT ON "+obj+" FROM reader")
	}

	if !hasGrant(planSQL, "plain") {
		t.Fatalf("plan missing GRANT on plain table:\n%s", planSQL)
	}
	if !hasGrant(planSQL, "events") {
		t.Fatalf("plan missing GRANT on partitioned table events (issue #640):\n%s", planSQL)
	}
	t.Logf("plan correctly includes GRANT on partitioned table events:\n%s", planSQL)

	if _, err := conn.ExecContext(ctx, `
		GRANT SELECT ON events TO reader;
		GRANT SELECT ON plain TO reader;
	`); err != nil {
		t.Fatalf("grant on live db: %v", err)
	}

	revokePlan, err := generatePlanSQLFormatted(host, port, dbname, user, password, schema, tablesFile)
	if err != nil {
		t.Fatalf("plan revoke: %v", err)
	}
	if !hasRevoke(revokePlan, "plain") {
		t.Fatalf("plan missing REVOKE on plain:\n%s", revokePlan)
	}
	if !hasRevoke(revokePlan, "events") {
		t.Fatalf("plan missing REVOKE on partitioned table events (issue #640):\n%s", revokePlan)
	}
	t.Logf("plan correctly includes REVOKE on partitioned table events:\n%s", revokePlan)
}
