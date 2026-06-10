package queries_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir/queries"
	"github.com/pgplex/pgschema/testutil"
)

var sharedTestPostgres *postgres.EmbeddedPostgres

func TestMain(m *testing.M) {
	sharedTestPostgres = testutil.SetupPostgres(nil)
	defer sharedTestPostgres.Stop()
	m.Run()
}

func TestGetSequencesForSchemaDetectsMixedCaseSequenceInColumnDefault(t *testing.T) {
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer conn.Close()

	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS orders CASCADE`); err != nil {
		t.Fatalf("failed to drop test table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE orders ("orderId" SERIAL PRIMARY KEY)`); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	// Drop the pg_depend ownership edge that SERIAL creates automatically.
	// GetSequencesForSchema detects ownership via two paths: pg_depend (primary)
	// and column_default parsing (fallback). Without this, pg_depend resolves
	// ownership before the column_default regex is ever reached, so the test
	// would pass even with the broken regex. OWNED BY NONE forces the fallback
	// path — the one that was broken for mixed-case identifiers before this fix.
	if _, err := conn.ExecContext(ctx, `ALTER SEQUENCE "orders_orderId_seq" OWNED BY NONE`); err != nil {
		t.Fatalf("failed to remove sequence ownership dependency: %v", err)
	}

	rows, err := queries.New(conn).GetSequencesForSchema(ctx, sql.NullString{String: "public", Valid: true})
	if err != nil {
		t.Fatalf("failed to get sequences for schema: %v", err)
	}

	for _, row := range rows {
		if row.SequenceName.String != "orders_orderId_seq" {
			continue
		}

		if !row.OwnedByTable.Valid || row.OwnedByTable.String != "orders" {
			t.Fatalf("OwnedByTable = %q, want %q", row.OwnedByTable.String, "orders")
		}
		if !row.OwnedByColumn.Valid || row.OwnedByColumn.String != "orderId" {
			t.Fatalf("OwnedByColumn = %q, want %q", row.OwnedByColumn.String, "orderId")
		}
		return
	}

	t.Fatalf("sequence %q not found; got sequences: %s", "orders_orderId_seq", sequenceNames(rows))
}

func sequenceNames(rows []queries.GetSequencesForSchemaRow) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.SequenceName.String)
	}
	return strings.Join(names, ", ")
}
