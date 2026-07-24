package queries_test

import (
	"context"
	"database/sql"
	"fmt"
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

// TestGetColumnsForSchemaDetectsExtensionOwnedTypeRegardlessOfSchema covers the
// extension-owned-type schema-qualifier fix: a column's data type can be owned
// by an extension installed in a schema other than "public" (citext/hstore are
// contrib modules bundled with the embedded-postgres binary; pgvector is not,
// so it isn't used here). GetColumnsForSchema must report the extension name
// regardless of which schema the extension lives in, and must report an empty
// extension name for ordinary (non-extension-owned) columns.
func TestGetColumnsForSchemaDetectsExtensionOwnedTypeRegardlessOfSchema(t *testing.T) {
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer conn.Close()

	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `DROP TABLE IF EXISTS docs CASCADE`); err != nil {
		t.Fatalf("failed to drop test table: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `DROP SCHEMA IF EXISTS ext_schema CASCADE`); err != nil {
		t.Fatalf("failed to drop test schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE SCHEMA ext_schema`); err != nil {
		t.Fatalf("failed to create test schema: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE EXTENSION IF NOT EXISTS hstore SCHEMA ext_schema`); err != nil {
		t.Fatalf("failed to create hstore extension: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE docs (id serial PRIMARY KEY, attrs ext_schema.hstore, plain_text text)`); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		conn.ExecContext(ctx, `DROP TABLE IF EXISTS docs CASCADE`)
		conn.ExecContext(ctx, `DROP SCHEMA IF EXISTS ext_schema CASCADE`)
	}()

	rows, err := queries.New(conn).GetColumnsForSchema(ctx, sql.NullString{String: "public", Valid: true})
	if err != nil {
		t.Fatalf("failed to get columns for schema: %v", err)
	}

	var foundAttrs, foundPlain bool
	for _, row := range rows {
		if fmt.Sprintf("%v", row.TableName) != "docs" {
			continue
		}
		switch fmt.Sprintf("%v", row.ColumnName) {
		case "attrs":
			foundAttrs = true
			if !row.ExtensionName.Valid || row.ExtensionName.String != "hstore" {
				t.Errorf("attrs column: ExtensionName = %+v, want valid %q", row.ExtensionName, "hstore")
			}
		case "plain_text":
			foundPlain = true
			if row.ExtensionName.Valid && row.ExtensionName.String != "" {
				t.Errorf("plain_text column: ExtensionName = %+v, want empty", row.ExtensionName)
			}
		}
	}
	if !foundAttrs {
		t.Fatalf("attrs column not found; got columns: %s", columnNames(rows))
	}
	if !foundPlain {
		t.Fatalf("plain_text column not found; got columns: %s", columnNames(rows))
	}
}

func columnNames(rows []queries.GetColumnsForSchemaRow) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, fmt.Sprintf("%v.%v", row.TableName, row.ColumnName))
	}
	return strings.Join(names, ", ")
}

func sequenceNames(rows []queries.GetSequencesForSchemaRow) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.SequenceName.String)
	}
	return strings.Join(names, ", ")
}
