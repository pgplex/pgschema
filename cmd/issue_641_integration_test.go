package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgschema/testutil"
)

// TestIssue641ConcurrentIndexWaitCrossSchema reproduces
// https://github.com/pgplex/pgschema/issues/641: an invalid index of the same
// name in another schema makes an unqualified wait query report done=false
// forever. The fixed wait query is schema-qualified and returns done=true for
// the target schema's valid index. An end-to-end apply on tenant_b also
// completes quickly despite the invalid peer in tenant_a.
func TestIssue641ConcurrentIndexWaitCrossSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	setup := `
CREATE SCHEMA tenant_a;
CREATE SCHEMA tenant_b;
CREATE TABLE tenant_a.items (sku int);
INSERT INTO tenant_a.items VALUES (1), (1);
CREATE TABLE tenant_b.items (sku int);
INSERT INTO tenant_b.items VALUES (1), (2);
`
	if _, err := conn.ExecContext(ctx, setup); err != nil {
		t.Fatalf("setup schemas: %v", err)
	}

	_, err := conn.ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY items_sku_idx ON tenant_a.items (sku)`)
	if err == nil {
		t.Fatal("expected unique violation creating concurrent index on tenant_a")
	}

	var aValid bool
	if err := conn.QueryRowContext(ctx, `
		SELECT i.indisvalid FROM pg_class c
		JOIN pg_namespace n ON c.relnamespace = n.oid
		JOIN pg_index i ON c.oid = i.indexrelid
		WHERE n.nspname = 'tenant_a' AND c.relname = 'items_sku_idx'
	`).Scan(&aValid); err != nil {
		t.Fatalf("inspect tenant_a index: %v", err)
	}
	if aValid {
		t.Fatal("expected tenant_a.items_sku_idx to be invalid")
	}

	// Pre-fix wait query (name only) — matches checkWaitStatus's first-row scan.
	unqualified := `
SELECT COALESCE(i.indisvalid, false) as done
FROM pg_class c
LEFT JOIN pg_index i ON c.oid = i.indexrelid
WHERE c.relname = 'items_sku_idx'
`
	var done bool
	if err := conn.QueryRowContext(ctx, unqualified).Scan(&done); err != nil {
		t.Fatalf("unqualified wait query: %v", err)
	}
	if done {
		t.Fatal("expected unqualified wait on invalid index to report done=false")
	}
	t.Log("reproduced issue #641 root cause: unqualified wait on invalid cross-schema index returns done=false (would hang apply)")

	if _, err := conn.ExecContext(ctx, `CREATE INDEX items_sku_idx ON tenant_b.items (sku)`); err != nil {
		t.Fatalf("create valid index on tenant_b: %v", err)
	}

	// With both indexes present, unqualified can return either row first.
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM pg_class c WHERE c.relname = 'items_sku_idx'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 indexes named items_sku_idx, got %d", n)
	}

	qualified := `
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'items_sku_idx'
  AND c.relnamespace = 'tenant_b'::regnamespace;
`
	var progress int
	if err := conn.QueryRowContext(ctx, qualified).Scan(&done, &progress); err != nil {
		t.Fatalf("qualified wait query: %v", err)
	}
	if !done {
		t.Fatal("schema-qualified wait query should report done=true for tenant_b")
	}
	t.Log("schema-qualified wait query reports done=true for tenant_b despite invalid peer in tenant_a")

	if _, err := conn.ExecContext(ctx, `DROP INDEX tenant_b.items_sku_idx`); err != nil {
		t.Fatalf("drop tenant_b index for apply: %v", err)
	}

	desired := `
CREATE TABLE items (sku int);
CREATE INDEX items_sku_idx ON tenant_b.items USING btree (sku);
`
	tmp := t.TempDir()
	desiredFile := filepath.Join(tmp, "desired.sql")
	if err := os.WriteFile(desiredFile, []byte(desired), 0644); err != nil {
		t.Fatal(err)
	}

	planSQL, err := generatePlanSQLFormatted(host, port, dbname, user, password, "tenant_b", desiredFile)
	if err != nil {
		t.Fatalf("plan tenant_b index: %v", err)
	}
	if !strings.Contains(planSQL, "CREATE INDEX") {
		t.Fatalf("expected index create in plan:\n%s", planSQL)
	}
	if strings.Contains(planSQL, "relname") && !strings.Contains(planSQL, "relnamespace = 'tenant_b'::regnamespace") {
		t.Fatalf("plan wait query missing schema qualification:\n%s", planSQL)
	}

	start := time.Now()
	if err := applySchemaChanges(host, port, dbname, user, password, "tenant_b", desiredFile); err != nil {
		t.Fatalf("apply tenant_b index with invalid peer in tenant_a: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 30*time.Second {
		t.Fatalf("apply took %v; likely hung on cross-schema invalid index (issue #641)", elapsed)
	}

	var bValid bool
	if err := conn.QueryRowContext(ctx, `
		SELECT i.indisvalid FROM pg_class c
		JOIN pg_namespace n ON c.relnamespace = n.oid
		JOIN pg_index i ON c.oid = i.indexrelid
		WHERE n.nspname = 'tenant_b' AND c.relname = 'items_sku_idx'
	`).Scan(&bValid); err != nil {
		t.Fatalf("inspect tenant_b after apply: %v", err)
	}
	if !bValid {
		t.Fatal("expected tenant_b.items_sku_idx valid after apply")
	}
	t.Logf("apply completed in %v with schema-qualified wait (issue #641 fixed)", elapsed)
}
