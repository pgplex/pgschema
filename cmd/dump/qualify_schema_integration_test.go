package dump

// Positive integration coverage for `dump --qualify-schema` type references (#493).
//
// Unlike the builder-level tests in internal/diff (which hand the builder a
// pre-qualified IR), this test drives the whole pipeline: pgdump SQL → embedded
// database → inspector → IR → dump. It proves the inspector now *preserves* the
// schema of same-schema user-defined type references for the four "Mechanism A"
// slices (column type, domain base type, composite attribute, aggregate state
// type), so --qualify-schema can emit them fully qualified — while the default
// (smart-qualification) dump keeps them bare.

import (
	"context"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

const qualifySchemaSetupSQL = `
CREATE TYPE color AS ENUM ('r', 'g', 'b');
-- column type: same-schema enum
CREATE TABLE swatch (
    id    integer PRIMARY KEY,
    shade color
);
-- domain base type: same-schema enum
CREATE DOMAIN color_domain AS color;
-- composite attribute: same-schema enum (plus a built-in that must stay bare)
CREATE TYPE money_amount AS (
    amount   numeric,
    currency color
);
-- aggregate state type: same-schema composite
CREATE TYPE acc AS (n integer);
CREATE FUNCTION acc_add(acc, integer) RETURNS acc
    LANGUAGE sql IMMUTABLE AS $$ SELECT ROW(($1).n + $2)::acc $$;
CREATE AGGREGATE mysum(integer) (SFUNC = acc_add, STYPE = acc);
`

func TestDumpCommand_QualifySchemaTypeReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	if _, err := conn.ExecContext(context.Background(), qualifySchemaSetupSQL); err != nil {
		t.Fatalf("Failed to set up schema: %v", err)
	}

	baseConfig := func(qualify bool) *DumpConfig {
		return &DumpConfig{
			Host:          host,
			Port:          port,
			DB:            dbname,
			User:          user,
			Password:      password,
			Schema:        "public",
			MultiFile:     false,
			File:          "",
			QualifySchema: qualify,
		}
	}

	// --qualify-schema: same-schema type references are fully qualified.
	qualified, err := ExecuteDump(baseConfig(true))
	if err != nil {
		t.Fatalf("qualified dump failed: %v", err)
	}
	for _, want := range []string{
		"shade public.color",    // column type
		"AS public.color",       // domain base type (CREATE DOMAIN ... AS public.color)
		"currency public.color", // composite attribute
		"STYPE = public.acc",    // aggregate state type
	} {
		if !strings.Contains(qualified, want) {
			t.Errorf("qualified dump missing %q\n---\n%s", want, qualified)
		}
	}
	// Built-in types must never be schema-qualified.
	if strings.Contains(qualified, "pg_catalog.") {
		t.Errorf("qualified dump must not qualify built-in types with pg_catalog:\n%s", qualified)
	}
	if !strings.Contains(qualified, "amount numeric") {
		t.Errorf("qualified dump should keep built-in composite attr type bare (amount numeric):\n%s", qualified)
	}

	// Default (smart qualification): the same references stay bare.
	def, err := ExecuteDump(baseConfig(false))
	if err != nil {
		t.Fatalf("default dump failed: %v", err)
	}
	for _, want := range []string{
		"shade color",
		"AS color",
		"currency color",
		"STYPE = acc",
	} {
		if !strings.Contains(def, want) {
			t.Errorf("default dump missing bare form %q\n---\n%s", want, def)
		}
	}
	if strings.Contains(def, "public.color") || strings.Contains(def, "public.acc") {
		t.Errorf("default dump must not qualify same-schema type references:\n%s", def)
	}
}
