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

// TestGetSequencesForSchemaDoesNotInferOwnershipFromNaming verifies that once
// a sequence's pg_depend ownership edge is explicitly removed (OWNED BY
// NONE), GetSequencesForSchema does NOT resurrect it by guessing from the
// sequence name or from column defaults - even when the sequence still
// happens to be named like PostgreSQL's implicit SERIAL convention
// ("<table>_<column>_seq") and is still referenced by that column's default.
// Ownership must come only from the database's own dependency graph: a
// naming-based fallback can misattribute ownership to a sequence that was
// never meant to be coupled to the table's lifecycle, causing it to be
// silently dropped if the table is ever dropped (see PR #575 review).
func TestGetSequencesForSchemaDoesNotInferOwnershipFromNaming(t *testing.T) {
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer conn.Close()

	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS orders CASCADE`); err != nil {
		t.Fatalf("failed to drop test table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE orders ("orderId" SERIAL PRIMARY KEY)`); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	// Remove the pg_depend ownership edge that SERIAL creates automatically,
	// while the column's default still references the sequence by name.
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

		if row.OwnedByTable.Valid || row.OwnedByColumn.Valid {
			t.Fatalf("expected no ownership after OWNED BY NONE, got OwnedByTable=%q OwnedByColumn=%q",
				row.OwnedByTable.String, row.OwnedByColumn.String)
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
