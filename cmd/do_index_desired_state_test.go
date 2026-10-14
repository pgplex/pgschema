package cmd

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestExternalDesiredStateDOIndexReferences(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded PostgreSQL integration")
	}
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, host, port, database, user, password := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	major, err := testutil.GetMajorVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []struct{ name, definition string }{
		{"idx", "CREATE INDEX idx ON public.t(id);"},
		{"idx", "CREATE UNIQUE INDEX IF NOT EXISTS idx ON ONLY public.t(id);"},
		{"idx", "CREATE INDEX idx ON public.t(id); CREATE INDEX IF NOT EXISTS idx ON public.outside_table(id);"},
		{"idx", "CREATE INDEX IF NOT EXISTS idx ON public.outside_table(id); CREATE UNIQUE INDEX idx ON public.t(id);"},
		{`Index."Quoted`, `CREATE INDEX "Index.""Quoted" ON public.t(id);`},
	} {
		for _, shadow := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/shadow=%v", index.definition, shadow), func(t *testing.T) {
				if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;
CREATE TABLE public.outside_table(id int);
CREATE INDEX untouched ON public.outside_table(id);
CREATE FUNCTION public.idx() RETURNS int LANGUAGE sql IMMUTABLE AS $$ SELECT 42 $$;`); err != nil {
					t.Fatal(err)
				}
				if shadow {
					if _, err := db.Exec("CREATE INDEX " + ir.QuoteIdentifier(index.name) + " ON public.outside_table(id)"); err != nil {
						t.Fatal(err)
					}
				}
				provider, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{
					Host: host, Port: port, Database: database, Username: user, Password: password,
					SSLMode: "disable", TargetMajorVersion: major,
				})
				if err != nil {
					t.Fatal(err)
				}
				defer provider.Stop()
				indexLiteral := "'public." + strings.ReplaceAll(ir.QuoteIdentifier(index.name), "'", "''") + "'"
				fixture := `CREATE TABLE public.t(id int);` + index.definition + `
CREATE TABLE public.resolved(index_id oid, untouched_id oid);
CREATE FUNCTION public.fetch_value() RETURNS int LANGUAGE sql AS $$ SELECT public.idx() $$;
CREATE VIEW public.result AS SELECT public.fetch_value() AS value;
DO $$ BEGIN INSERT INTO resolved VALUES (` + indexLiteral + `::pg_catalog.regclass, 'public.untouched'::regclass); END $$;`
				if err := provider.ApplySchema(context.Background(), "public", fixture); err != nil {
					t.Fatal(err)
				}
				var indexSchema, tableName, untouchedSchema, untouchedTable string
				err = db.QueryRow(`SELECT n.nspname, t.relname, un.nspname, ut.relname
FROM `+ir.QuoteIdentifier(provider.GetSchemaName())+`.resolved r
JOIN pg_catalog.pg_class c ON c.oid=r.index_id
JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
JOIN pg_catalog.pg_index i ON i.indexrelid=c.oid
JOIN pg_catalog.pg_class t ON t.oid=i.indrelid
JOIN pg_catalog.pg_class uc ON uc.oid=r.untouched_id
JOIN pg_catalog.pg_namespace un ON un.oid=uc.relnamespace
JOIN pg_catalog.pg_index ui ON ui.indexrelid=uc.oid
JOIN pg_catalog.pg_class ut ON ut.oid=ui.indrelid`).Scan(&indexSchema, &tableName, &untouchedSchema, &untouchedTable)
				if err != nil {
					t.Fatal(err)
				}
				if indexSchema != provider.GetSchemaName() || tableName != "t" {
					t.Fatalf("created index resolved to %s.%s", indexSchema, tableName)
				}
				if untouchedSchema != "public" || untouchedTable != "outside_table" {
					t.Fatalf("unmanaged index resolved to %s.%s", untouchedSchema, untouchedTable)
				}
				var value int
				if err := db.QueryRow("SELECT value FROM " + ir.QuoteIdentifier(provider.GetSchemaName()) + ".result").Scan(&value); err != nil {
					t.Fatal(err)
				}
				if value != 42 {
					t.Fatalf("external function reference changed: %d", value)
				}
			})
		}
	}
}
