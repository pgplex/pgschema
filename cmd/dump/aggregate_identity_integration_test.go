package dump

import (
	"context"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

// TestAggregateIdentityArgsQuotedSchema verifies that an aggregate over a
// relation row type inspects with unqualified identity arguments even when its
// schema needs quoting. The inspecting connection does not have the schema on
// its search_path, so PostgreSQL renders the type as "MySchema".v; both that
// and the public.v form must normalize to v, or every plan would drop and
// recreate the aggregate (#580 review).
func TestAggregateIdentityArgsQuotedSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	ctx := context.Background()
	setup := `
CREATE SCHEMA "MySchema";
CREATE TABLE "MySchema".t (id integer);
CREATE VIEW "MySchema".v AS SELECT id FROM "MySchema".t;
CREATE FUNCTION "MySchema".v_step(state integer, r "MySchema".v) RETURNS integer
    LANGUAGE sql IMMUTABLE AS $$ SELECT state + r.id $$;
CREATE AGGREGATE "MySchema".sum_v("MySchema".v) (SFUNC = "MySchema".v_step, STYPE = integer, INITCOND = '0');
`
	if _, err := conn.ExecContext(ctx, setup); err != nil {
		t.Fatalf("Failed to set up schema: %v", err)
	}

	schemaIR, err := ir.NewInspector(conn, nil).BuildIR(ctx, "MySchema")
	if err != nil {
		t.Fatalf("BuildIR failed: %v", err)
	}
	dbSchema := schemaIR.Schemas["MySchema"]
	if dbSchema == nil {
		t.Fatalf("schema \"MySchema\" not found in IR; got %v", schemaIR.Schemas)
	}
	agg, ok := dbSchema.Aggregates["sum_v(v)"]
	if !ok {
		var keys []string
		for k := range dbSchema.Aggregates {
			keys = append(keys, k)
		}
		t.Fatalf("expected aggregate key %q, got %v", "sum_v(v)", keys)
	}
	if agg.Arguments != "v" || agg.Signature != "v" {
		t.Errorf("expected Arguments and Signature %q, got Arguments=%q Signature=%q", "v", agg.Arguments, agg.Signature)
	}
}
