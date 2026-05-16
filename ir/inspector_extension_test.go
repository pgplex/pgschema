package ir_test

import (
	"testing"

	"github.com/pgplex/pgschema/testutil"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestExtensionDetection(t *testing.T) {
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	setupSQL := "CREATE EXTENSION IF NOT EXISTS btree_gist;"

	t.Run("empty schema does not detect extensions", func(t *testing.T) {
		ir := testutil.ParseSQLToIRWithSetup(t, embeddedPG, "-- empty", "public", setupSQL)
		if len(ir.Extensions) != 0 {
			t.Errorf("Expected no extensions for empty schema, got %v", ir.Extensions)
		}
	})

	t.Run("schema with EXCLUDE constraint detects btree_gist", func(t *testing.T) {
		sql := `
CREATE TABLE reservations (
    id uuid,
    resource_id uuid NOT NULL,
    start_date date NOT NULL,
    end_date date NOT NULL,
    CONSTRAINT reservations_pkey PRIMARY KEY (id),
    CONSTRAINT no_overlap EXCLUDE USING gist (resource_id WITH =, daterange(start_date, end_date, '[]'::text) WITH &&)
);`
		ir := testutil.ParseSQLToIRWithSetup(t, embeddedPG, sql, "public", setupSQL)
		if len(ir.Extensions) != 1 || ir.Extensions[0] != "btree_gist" {
			t.Errorf("Expected [btree_gist], got %v", ir.Extensions)
		}
	})
}
