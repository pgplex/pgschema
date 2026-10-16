package cmd

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestDesiredStateUnicode(t *testing.T) {
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
	externalProvider := func(t *testing.T) postgres.DesiredStateProvider {
		t.Helper()
		external, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{Host: h, Port: p, Database: d, Username: u, Password: w, SSLMode: "disable", TargetMajorVersion: major})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { external.Stop() })
		return external
	}
	readMarker := func(t *testing.T, conn *sql.DB, schema string) string {
		t.Helper()
		var got string
		if err := conn.QueryRow("SELECT v FROM " + ir.QuoteIdentifier(schema) + ".marker").Scan(&got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	for _, tc := range []struct{ name, schema, sql, want string }{
		{"unicode_schema", "public", `CREATE TABLE U&"public".t(id int);
CREATE TABLE public.marker(v text);
DO U&'BEGIN IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=''public.t''::regclass) <> current_schema() THEN RAISE EXCEPTION ''wrong namespace''; END IF; INSERT INTO marker VALUES (''ok''); END';`, "ok"},
		{"escaped_names_and_body", "public", `CREATE TABLE u&"!0070ublic" /* schema comment */ UESCAPE '!' . U&"t\+01F600"(id int);
CREATE UNIQUE INDEX U&"i!D83D!DE00" UESCAPE '!' ON U&"\0070ublic".U&"t\D83D\DE00"(id);
CREATE TABLE public.marker(v text);
DO u&'#0042EGIN
IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=U&''!0070ublic."t!+01F600"'' /* regclass comment */ UESCAPE ''!''::regclass) <> current_schema() THEN RAISE EXCEPTION ''wrong table namespace''; END IF;
IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=CAST(U&''!0070ublic."i!D83D!DE00"'' UESCAPE ''!'' AS pg_catalog.regclass)) <> current_schema() THEN RAISE EXCEPTION ''wrong index namespace''; END IF;
EXECUTE format(''INSERT INTO %s VALUES (%L)'', ''public.marker''::regclass, U&''!0070ublic.t!!'' UESCAPE ''!'');
END' /* body comment */ UESCAPE '#';`, "public.t!"},
		{"encoded_target_schema", "schéma", `CREATE TABLE U&"sch!00E9ma" UESCAPE '!' . U&"t\00E9"(id int);
CREATE TABLE U&"sch\00E9ma".marker(v text);
DO $$ BEGIN
IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=U&'sch\00E9ma.t\00E9'::regclass) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF;
EXECUTE format('INSERT INTO %s VALUES (%L)', U&'sch!00E9ma.marker' UESCAPE '!'::regclass, 'ok'); END $$;`, "ok"},
		{"decoded_dollar_delimiter", "public", `CREATE TABLE public."t$body$"(id int);
CREATE TABLE public.marker(v text);
DO $body$ BEGIN
IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=U&'public."t\0024body\0024"'::regclass) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF;
INSERT INTO marker VALUES ('ok'); END $body$;`, "ok"},
		{"unicode_inner_literal", "public", `CREATE TABLE public.marker(v text);
DO U&'\0042EGIN EXECUTE format(''INSERT INTO %s VALUES (%L)'', U&''\\0070ublic.marker''::regclass, ''ok''); END';`, "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quoted := ir.QuoteIdentifier(tc.schema)
			if _, err := db.Exec("DROP SCHEMA IF EXISTS " + quoted + " CASCADE; CREATE SCHEMA " + quoted + "; SET search_path = " + quoted + ", pg_catalog; " + tc.sql); err != nil {
				t.Fatalf("native PostgreSQL fixture: %v", err)
			}
			if got := readMarker(t, db, tc.schema); got != tc.want {
				t.Fatalf("native marker %q, want %q", got, tc.want)
			}
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": externalProvider(t)} {
				t.Run(name, func(t *testing.T) {
					if err := provider.ApplySchema(context.Background(), tc.schema, tc.sql); err != nil {
						t.Fatal(err)
					}
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					if got := readMarker(t, conn, provider.GetSchemaName()); got != tc.want {
						t.Fatalf("temporary marker %q, want %q", got, tc.want)
					}
				})
			}
			if got := readMarker(t, db, tc.schema); got != tc.want {
				t.Fatalf("original marker changed to %q", got)
			}
		})
	}
	for _, tc := range []struct{ literal, want string }{
		{`U&'\D83D\DE00'`, "😀"}, {`U&'\D83D\+00DE00'`, "😀"}, {`U&'\+01F600'`, "😀"},
		{`u&'it''s \\ text'`, "it's \\ text"}, {`U&'!0061!!' UESCAPE E'!'`, "a!"}, {`U&'!0061' UESCAPE $$!$$`, "a"},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			var native string
			if err := db.QueryRow("SELECT " + tc.literal).Scan(&native); err != nil || native != tc.want {
				t.Fatalf("native value %q, %v; want %q", native, err, tc.want)
			}
			fixture := `CREATE TABLE public.marker(v text); DO $body$ BEGIN EXECUTE format('INSERT INTO %s VALUES (%L)', 'public.marker'::regclass, ` + tc.literal + `); END $body$;`
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": externalProvider(t)} {
				t.Run(name, func(t *testing.T) {
					if err := provider.ApplySchema(context.Background(), "public", fixture); err != nil {
						t.Fatal(err)
					}
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					if got := readMarker(t, conn, provider.GetSchemaName()); got != native {
						t.Fatalf("marker %q, native %q", got, native)
					}
				})
			}
		})
	}
	for _, bad := range []string{
		`U&'\12'`, `U&'\12GG'`, `U&'\+110000'`, `U&'\0000'`, `U&'\D83D'`, `U&'\DE00'`, `U&'\D83D\0041'`, `U&'\D83D\DE'`,
		`U&'a' UESCAPE ''`, `U&'a' UESCAPE 'ab'`, `U&'a' UESCAPE '+'`, `U&'a' UESCAPE '0'`, `U&'a' UESCAPE 'F'`, `U&'a' UESCAPE ' '`, `U&'a' UESCAPE '"'`, `U&'a' UESCAPE ''''`, `U&'a' UESCAPE '§'`, `U&'a' UESCAPE U&'!'`, `U&'a' UESCAPE`, `U&'a' UESCAPE E'\u12'`,
		`U&'unfinished`, `U&"unfinished`,
	} {
		t.Run("invalid_"+bad, func(t *testing.T) {
			_, err := db.Exec("SELECT " + bad)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("native PostgreSQL accepted invalid Unicode: %v", err)
			}
			// Exercise each boundary, including decoding inside an executable DO body.
			for _, fixture := range []string{`CREATE TABLE public.marker(v text); SELECT ` + bad, `CREATE TABLE public.marker(v text); DO $$ BEGIN PERFORM ` + bad + `; END $$;`} {
				for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": externalProvider(t)} {
					t.Run(name, func(t *testing.T) {
						if err := provider.ApplySchema(context.Background(), "public", fixture); err == nil {
							t.Fatal("provider accepted invalid Unicode")
						}
						conn := db
						if name == "embedded" {
							conn = embeddedDB
						}
						var count int
						if err := conn.QueryRow(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname='marker'`, provider.GetSchemaName()).Scan(&count); err != nil {
							t.Fatal(err)
						}
						if count != 0 {
							t.Fatal("provider executed SQL before Unicode decoding failed")
						}
					})
				}
			}
		})
	}
}
