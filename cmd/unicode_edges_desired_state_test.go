package cmd

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestDesiredStateUnicodeEdges(t *testing.T) {
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
	const prefix = `CREATE TABLE public.t(id int); CREATE TABLE public.marker(v text); `
	const body = `BEGIN IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid='public.t'::regclass) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF; INSERT INTO marker VALUES ('ok'); END`
	literal := func(text string) string { return "'" + strings.ReplaceAll(text, "'", "''") + "'" }
	for _, tc := range []struct{ name, sql, state string }{
		{"continued_suffix", prefix + "DO U&" + literal(strings.TrimSuffix(body, "END")) + "\n'\\0045ND';", ""},
		{"continued_custom_suffix", prefix + "DO U&" + literal(strings.TrimSuffix(body, "END")) + "\n'!0045ND' UESCAPE '!';", ""},
		{"continued_prefix", prefix + "DO U&'BEGIN'\n" + literal(strings.TrimPrefix(body, "BEGIN")) + ";", ""},
		{"continued_line_comment", prefix + "DO U&'BEGIN'-- keep public.t\n" + literal(strings.TrimPrefix(body, "BEGIN")) + ";", ""},
		{"continued_line_comment_cr", prefix + "DO U&'BEGIN'-- keep public.t\r" + literal(strings.TrimPrefix(body, "BEGIN")) + ";", ""},
		{"continued_cr", prefix + "DO U&'BEGIN'\r" + literal(strings.TrimPrefix(body, "BEGIN")) + ";", ""},
		{"continued_regclass", prefix + `DO $$ BEGIN IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=U&'public.'
'!0074' UESCAPE '!'::regclass) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF; INSERT INTO marker VALUES ('ok'); END $$;`, ""},
		{"continued_surrogate", `CREATE TABLE public."t😀"(id int); CREATE TABLE public.marker(v text); DO $$ BEGIN IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=U&'public."t\D83D'
'\DE00"'::regclass) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF; INSERT INTO marker VALUES ('ok'); END $$;`, ""},
		{"unusual_names", `CREATE TABLE public."tÉ"(id int); CREATE TABLE public."t!"(id int); CREATE TABLE public."t/*x*/"(id int); CREATE TABLE public."t\x'"(id int); CREATE TABLE public."t""q"(id int); CREATE TABLE public."u&""t"""(id int); CREATE TABLE public.marker(v text);
DO $$ DECLARE rel regclass; BEGIN FOREACH rel IN ARRAY ARRAY['public.TÉ'::regclass, 'public.t!'::regclass, 'public.t/*x*/'::regclass, U&'public."t\\x''"'::regclass, 'public."t""q"'::regclass, 'public.U&"t"'::regclass] LOOP
IF (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=rel) <> current_schema() THEN RAISE EXCEPTION 'wrong namespace'; END IF; END LOOP; INSERT INTO marker VALUES ('ok'); END $$;`, ""},
		{"invalid_utf8_comment", prefix + "DO U&'BEGIN PERFORM ''public.t''::regclass; -- \xff\nEND';", "22021"},
		{"invalid_utf8_string", prefix + "DO U&'BEGIN PERFORM ''public.t''::regclass; RAISE NOTICE ''\xff''; END';", "22021"},
		{"invalid_utf8_split", prefix + "DO U&'BEGIN PERFORM ''public.t''::regclass; -- \xc3'\n'\xa9\nEND';", "22021"},
		{"invalid_utf8_uescape", prefix + "DO U&'BEGIN PERFORM ''public.t''::regclass; END' UESCAPE '\xff';", "22021"},
		{"invalid_utf8_escaped_uescape", prefix + `DO U&'BEGIN PERFORM ''public.t''::regclass; END' UESCAPE E'\377';`, "22021"},
		{"regclass_not_sql", prefix + `DO $$ BEGIN PERFORM 'U&"public".t'::regclass; END $$;`, "42P01"},
		{"standard_conforming_off", prefix + `DO $$ BEGIN SET standard_conforming_strings=off; PERFORM U&'public.t'::regclass; END $$;`, "0A000"},
		{"standard_conforming_off_custom", prefix + `DO $$ BEGIN SET standard_conforming_strings=off; PERFORM U&'!0070ublic.t' /*keep*/ UESCAPE '!'::regclass; END $$;`, "0A000"},
		{"space_is_not_continuation", prefix + "DO U&" + literal(body) + " 'END';", "42601"},
		{"block_comment_is_not_continuation", prefix + "DO U&" + literal(body) + " /*keep*/\n'END';", "42601"},
		{"block_comment_after_newline", prefix + "DO U&" + literal(body) + "\n/*keep*/'END';", "42601"},
		{"continued_uescape_multichar", prefix + "DO U&" + literal(body) + " UESCAPE '!'\n'?';", "42601"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(`SET standard_conforming_strings=on; DROP SCHEMA public CASCADE; CREATE SCHEMA public; SET search_path=public,pg_catalog;`); err != nil {
				t.Fatal(err)
			}
			_, nativeErr := db.Exec(tc.sql)
			expectedState := tc.state
			// PostgreSQL 14 reports a missing schema for this regclass name.
			// Match each provider against the independently observed native code.
			if tc.name == "regclass_not_sql" {
				var pgErr *pgconn.PgError
				if errors.As(nativeErr, &pgErr) && pgErr.Code == "3F000" {
					expectedState = pgErr.Code
				}
			}
			assertState := func(err error) {
				t.Helper()
				var pgErr *pgconn.PgError
				if !errors.As(err, &pgErr) || pgErr.Code != expectedState {
					t.Fatalf("database error %v, want SQLSTATE %s", err, expectedState)
				}
			}
			if tc.state == "" {
				if nativeErr != nil {
					t.Fatalf("native fixture: %v", nativeErr)
				}
			} else {
				assertState(nativeErr)
			}
			external, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{Host: h, Port: p, Database: d, Username: u, Password: w, SSLMode: "disable", TargetMajorVersion: major})
			if err != nil {
				t.Fatal(err)
			}
			defer external.Stop()
			readMarker := func(conn *sql.DB, schema string) string {
				t.Helper()
				var got string
				if err := conn.QueryRow("SELECT v FROM " + ir.QuoteIdentifier(schema) + ".marker").Scan(&got); err != nil {
					t.Fatal(err)
				}
				return got
			}
			for name, provider := range map[string]postgres.DesiredStateProvider{"embedded": sharedEmbeddedPG, "external": external} {
				t.Run(name, func(t *testing.T) {
					err := provider.ApplySchema(context.Background(), "public", tc.sql)
					conn := db
					if name == "embedded" {
						conn = embeddedDB
					}
					if tc.state == "" {
						if err != nil {
							t.Fatal(err)
						}
						if got := readMarker(conn, provider.GetSchemaName()); got != "ok" {
							t.Fatalf("temporary marker %q", got)
						}
					} else {
						if err == nil {
							t.Fatal("provider accepted native-invalid SQL")
						}
						// UTF-8 validation fails locally. Other failures retain PostgreSQL's
						// SQLSTATE, including Unicode rejection after the DO block changes GUCs.
						if tc.state != "22021" && tc.name != "continued_uescape_multichar" {
							assertState(err)
						}
						var count int
						if err := conn.QueryRow(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname='marker'`, provider.GetSchemaName()).Scan(&count); err != nil {
							t.Fatal(err)
						}
						if count != 0 {
							t.Fatal("native-invalid script left a marker")
						}
					}
				})
			}
			if tc.state == "" && readMarker(db, "public") != "ok" {
				t.Fatal("original marker changed")
			}
		})
	}
}
