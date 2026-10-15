package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgschema/testutil"
)

// TestIssue641ConcurrentIndexWaitCrossSchema reproduces
// https://github.com/pgplex/pgschema/issues/641: an invalid index of the same
// name in another schema made the CREATE INDEX CONCURRENTLY wait query, which
// matched by index name only, report done=false forever. The wait query is now
// schema-qualified by exact nspname, so apply completes for the target schema,
// including mixed-case quoted schema names.
func TestIssue641ConcurrentIndexWaitCrossSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// A failed concurrent unique build leaves an invalid tenant_a.items_sku_idx.
	if _, err := conn.ExecContext(ctx, `
CREATE SCHEMA tenant_a;
CREATE TABLE tenant_a.items (sku int);
INSERT INTO tenant_a.items VALUES (1), (1);
`); err != nil {
		t.Fatalf("setup tenant_a: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY items_sku_idx ON tenant_a.items (sku)`); err == nil {
		t.Fatal("expected unique violation creating concurrent index on tenant_a")
	}
	if valid := indexValid(t, conn, "tenant_a", "items_sku_idx"); valid {
		t.Fatal("expected tenant_a.items_sku_idx to be invalid")
	}

	// The pre-fix wait query (index name only) reports done=false: apply hangs.
	var done bool
	if err := conn.QueryRowContext(ctx, `
SELECT COALESCE(i.indisvalid, false) as done
FROM pg_class c
LEFT JOIN pg_index i ON c.oid = i.indexrelid
WHERE c.relname = 'items_sku_idx'`).Scan(&done); err != nil {
		t.Fatalf("unqualified wait query: %v", err)
	}
	if done {
		t.Fatal("expected name-only wait query on invalid index to report done=false")
	}

	for _, schema := range []string{"tenant_b", "TenantB"} {
		t.Run(schema, func(t *testing.T) {
			quoted := `"` + schema + `"`
			if _, err := conn.ExecContext(ctx, fmt.Sprintf(`
CREATE SCHEMA %[1]s;
CREATE TABLE %[1]s.items (sku int);
INSERT INTO %[1]s.items VALUES (1), (2);`, quoted)); err != nil {
				t.Fatalf("setup %s: %v", schema, err)
			}

			desiredFile := filepath.Join(t.TempDir(), "desired.sql")
			desired := `
CREATE TABLE items (sku int);
CREATE INDEX items_sku_idx ON items USING btree (sku);
`
			if err := os.WriteFile(desiredFile, []byte(desired), 0644); err != nil {
				t.Fatal(err)
			}

			planSQL, err := generatePlanSQLFormatted(host, port, dbname, user, password, schema, desiredFile)
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			wantFilter := "n.nspname = '" + schema + "'"
			if !strings.Contains(planSQL, "CREATE INDEX CONCURRENTLY") || !strings.Contains(planSQL, wantFilter) {
				t.Fatalf("expected concurrent index with wait filter %q in plan:\n%s", wantFilter, planSQL)
			}

			// Fail rather than hang if the wait directive never completes.
			errCh := make(chan error, 1)
			go func() {
				errCh <- applySchemaChanges(host, port, dbname, user, password, schema, desiredFile)
			}()
			select {
			case err := <-errCh:
				if err != nil {
					t.Fatalf("apply with invalid peer index in tenant_a: %v", err)
				}
			case <-time.After(60 * time.Second):
				t.Fatal("apply did not finish within 60s; wait directive likely matched tenant_a's invalid index (issue #641)")
			}

			if !indexValid(t, conn, schema, "items_sku_idx") {
				t.Fatalf("expected %s.items_sku_idx valid after apply", schema)
			}
		})
	}
}

func indexValid(t *testing.T, conn *sql.DB, schema, index string) bool {
	t.Helper()
	var valid bool
	if err := conn.QueryRow(`
		SELECT i.indisvalid FROM pg_class c
		JOIN pg_namespace n ON c.relnamespace = n.oid
		JOIN pg_index i ON c.oid = i.indexrelid
		WHERE n.nspname = $1 AND c.relname = $2`, schema, index).Scan(&valid); err != nil {
		t.Fatalf("inspect %s.%s: %v", schema, index, err)
	}
	return valid
}
