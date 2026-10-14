package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

// Compare the loader with PostgreSQL's own literal semantics on an owned local
// instance. Invalid SQL must fail before executing the desired-state script.
func TestDesiredStateDOLiteralSemantics(t *testing.T) {
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
	external, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{Host: h, Port: p, Database: d, Username: u, Password: w, SSLMode: "disable", TargetMajorVersion: major})
	if err != nil {
		t.Fatal(err)
	}
	defer external.Stop()
	embeddedDB, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedEmbeddedPG)
	defer embeddedDB.Close()
	for _, tc := range []struct{ name, body string }{
		{"incomplete_u", `DO E'BEGIN RAISE NOTICE ''\u12''; END';`},
		{"incomplete_U", `DO E'BEGIN RAISE NOTICE ''\U0001F60''; END';`},
		{"invalid_hex", `DO E'BEGIN RAISE NOTICE ''\u12GG''; END';`},
		{"out_of_range", `DO E'BEGIN RAISE NOTICE ''\U00110000''; END';`},
		{"maximum_uint32", `DO E'BEGIN RAISE NOTICE ''\UFFFFFFFF''; END';`},
		{"zero", `DO E'BEGIN RAISE NOTICE ''\u0000''; END';`},
		{"unpaired_low_surrogate", `DO E'BEGIN RAISE NOTICE ''\uDE00''; END';`},
		{"invalid_low_surrogate", `DO E'BEGIN RAISE NOTICE ''\uD83D\u0041''; END';`},
		{"unpaired_surrogate", `DO E'BEGIN RAISE NOTICE ''\uD83D''; END';`},
		{"invalid_regclass", `DO $$ BEGIN PERFORM E'public.\u12'::pg_catalog.regclass; END $$;`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, rawErr := db.Exec(tc.body)
			var pgErr *pgconn.PgError
			if !errors.As(rawErr, &pgErr) {
				t.Fatalf("PostgreSQL accepted invalid fixture: %v", rawErr)
			}
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": external} {
				t.Run(name, func(t *testing.T) {
					err := provider.ApplySchema(context.Background(), "public", `CREATE TABLE public.marker(v text); `+tc.body)
					if err == nil || !strings.Contains(err.Error(), "failed to remap desired state SQL") {
						t.Fatalf("missing desired-state remapping error: %v", err)
					}
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					var count int
					if err := conn.QueryRow(`SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname='marker'`, provider.GetSchemaName()).Scan(&count); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatal("invalid desired SQL executed before remapping failed")
					}
				})
			}
		})
	}
	for _, escape := range []string{`\uD83D\uDE00`, `\U0000D83D\uDE00`, `\uD83D\U0000DE00`, `\U0001F600`} {
		t.Run(escape, func(t *testing.T) {
			const prefix = `CREATE TABLE public.marker(v text); `
			body := `DO E'BEGIN EXECUTE pg_catalog.format(''INSERT INTO %s VALUES (%L)'', ''public.marker''::pg_catalog.regclass, ''` + escape + `''); END';`
			if _, err := db.Exec(`DROP TABLE IF EXISTS public.marker; ` + prefix + body); err != nil {
				t.Fatal(err)
			}
			var want string
			if err := db.QueryRow(`SELECT v FROM public.marker`).Scan(&want); err != nil {
				t.Fatal(err)
			}
			if want != "😀" {
				t.Fatalf("unexpected native Unicode value %q", want)
			}
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": external} {
				t.Run(name, func(t *testing.T) {
					if name == "external" {
						if _, err := db.Exec("DROP TABLE IF EXISTS " + ir.QuoteIdentifier(provider.GetSchemaName()) + ".marker"); err != nil {
							t.Fatal(err)
						}
					}
					if err := provider.ApplySchema(context.Background(), "public", prefix+body); err != nil {
						t.Fatal(err)
					}
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					var got string
					if err := conn.QueryRow("SELECT v FROM " + ir.QuoteIdentifier(provider.GetSchemaName()) + ".marker").Scan(&got); err != nil {
						t.Fatal(err)
					}
					if got != want {
						t.Fatalf("loader changed Unicode content: got %q, want %q", got, want)
					}
				})
			}
		})
	}
}
