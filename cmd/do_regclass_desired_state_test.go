package cmd

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestDesiredStateDOAliasesAndRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded PostgreSQL integration")
	}
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, h, p, d, u, w := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	major, err := testutil.GetMajorVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	embeddedDB, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedEmbeddedPG)
	defer embeddedDB.Close()
	const prefix = `CREATE TABLE public.t(t int);
INSERT INTO t VALUES (7);
CREATE TABLE public.marker(v text);
`
	for _, tc := range []struct{ name, body, want string }{
		{"alias", `DO $$ BEGIN INSERT INTO marker SELECT public.t::text FROM t AS public; END $$;`, "7"},
		{"quoted_alias", `DO $$ BEGIN INSERT INTO marker SELECT "public".t::text FROM t AS "public"; END $$;`, "7"},
		{"record", `DO $$ DECLARE public record; BEGIN SELECT 9 AS t INTO public; INSERT INTO marker VALUES (public.t::text); END $$;`, "9"},
		{"alias_and_regclass", `DO $$ BEGIN INSERT INTO marker SELECT public.t::text FROM t AS public WHERE 'public.t'::pg_catalog.regclass = 't'::regclass; END $$;`, "7"},
		{"record_and_regclass", `DO $$ DECLARE public record; BEGIN SELECT 9 AS t INTO public; IF CAST('public.t' AS pg_catalog.regclass) = 't'::regclass THEN INSERT INTO marker VALUES (public.t::text); END IF; END $$;`, "9"},
		{"nested_record", `DO $$ DECLARE public record; BEGIN
SELECT 11 AS t INTO public;
INSERT INTO marker VALUES (public.t::text);
DECLARE public record; BEGIN
SELECT 13 AS t INTO public;
INSERT INTO marker VALUES (public.t::text);
END;
INSERT INTO marker VALUES (public.t::text);
END $$;`, "11,11,13"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := prefix + tc.body
			if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public; ` + fixture); err != nil {
				t.Fatalf("independent PostgreSQL fixture failed: %v", err)
			}
			readMarker := func(conn *sql.DB, schema string) string {
				t.Helper()
				var got string
				if err := conn.QueryRow("SELECT COALESCE(string_agg(v, ',' ORDER BY v), '') FROM " + ir.QuoteIdentifier(schema) + ".marker").Scan(&got); err != nil {
					t.Fatal(err)
				}
				return got
			}
			if got := readMarker(db, "public"); got != tc.want {
				t.Fatalf("native PostgreSQL result = %q, want %q", got, tc.want)
			}
			external, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{Host: h, Port: p, Database: d, Username: u, Password: w, SSLMode: "disable", TargetMajorVersion: major})
			if err != nil {
				t.Fatal(err)
			}
			defer external.Stop()
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": external} {
				t.Run(name, func(t *testing.T) {
					if err := provider.ApplySchema(context.Background(), "public", fixture); err != nil {
						t.Fatal(err)
					}
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					if got := readMarker(conn, provider.GetSchemaName()); got != tc.want {
						t.Fatalf("loader result = %q, want %q", got, tc.want)
					}
				})
			}
			if got := readMarker(db, "public"); got != tc.want {
				t.Fatalf("loader changed the original schema: %q", got)
			}
		})
	}
}
