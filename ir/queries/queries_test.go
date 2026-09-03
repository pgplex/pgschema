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

func TestGetSequencesForSchemaOwnershipComesFromPgDepend(t *testing.T) {
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer conn.Close()

	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS orders CASCADE`); err != nil {
		t.Fatalf("failed to drop test table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE orders ("orderId" SERIAL PRIMARY KEY)`); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	findSeq := func() queries.GetSequencesForSchemaRow {
		rows, err := queries.New(conn).GetSequencesForSchema(ctx, sql.NullString{String: "public", Valid: true})
		if err != nil {
			t.Fatalf("failed to get sequences for schema: %v", err)
		}
		for _, row := range rows {
			if row.SequenceName.String == "orders_orderId_seq" {
				return row
			}
		}
		t.Fatalf("sequence %q not found; got sequences: %s", "orders_orderId_seq", sequenceNames(rows))
		return queries.GetSequencesForSchemaRow{}
	}

	// SERIAL records ownership in pg_depend, including mixed-case column names.
	row := findSeq()
	if row.OwnedByTable.String != "orders" {
		t.Fatalf("OwnedByTable = %q, want %q", row.OwnedByTable.String, "orders")
	}
	if row.OwnedByColumn.String != "orderId" {
		t.Fatalf("OwnedByColumn = %q, want %q", row.OwnedByColumn.String, "orderId")
	}

	// Once the pg_depend edge is gone, the sequence is unowned even though the
	// column default still references it. Ownership must not be inferred from
	// column defaults (issue #573).
	if _, err := conn.ExecContext(ctx, `ALTER SEQUENCE "orders_orderId_seq" OWNED BY NONE`); err != nil {
		t.Fatalf("failed to remove sequence ownership dependency: %v", err)
	}
	row = findSeq()
	if row.OwnedByTable.String != "" || row.OwnedByColumn.String != "" {
		t.Fatalf("expected no ownership after OWNED BY NONE, got table=%q column=%q", row.OwnedByTable.String, row.OwnedByColumn.String)
	}
}

func sequenceNames(rows []queries.GetSequencesForSchemaRow) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.SequenceName.String)
	}
	return strings.Join(names, ", ")
}
