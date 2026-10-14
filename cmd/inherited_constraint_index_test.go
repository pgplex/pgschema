package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/cmd/apply"
	"github.com/pgplex/pgschema/cmd/dump"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/internal/include"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

func TestInheritedConstraintIndexRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded PostgreSQL integration")
	}
	t.Chdir(t.TempDir())
	source := testutil.SetupPostgres(t)
	defer source.Stop()
	sourceDB, sh, sp, sd, su, sw := testutil.ConnectToPostgres(t, source)
	defer sourceDB.Close()
	majorVersion, err := testutil.GetMajorVersion(sourceDB)
	if err != nil {
		t.Fatal(err)
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	targetDB, h, p, d, u, w := testutil.ConnectToPostgres(t, target)
	defer targetDB.Close()
	fixtures := []struct{ name, schema, sql string }{
		{"reported_primary_key", "public", `CREATE TABLE public.events (id int PRIMARY KEY) PARTITION BY RANGE(id);
CREATE INDEX events_leaf_pkey ON public.events(id);
CREATE TABLE public.events_leaf PARTITION OF public.events FOR VALUES FROM (0) TO (10);`},
		{"unique_collision", "public", `CREATE TABLE events (id int UNIQUE) PARTITION BY RANGE(id);
CREATE INDEX events_leaf_id_key ON events(id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);`},
		{"name_swap", "public", `CREATE TABLE events (id int, a int, b int, UNIQUE(id,a), UNIQUE(id,b)) PARTITION BY RANGE(id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);
ALTER INDEX events_leaf_id_a_key RENAME TO staging;
ALTER INDEX events_leaf_id_b_key RENAME TO events_leaf_id_a_key;
ALTER INDEX staging RENAME TO events_leaf_id_b_key;`},
		{"nested_local_constraints", "public", `CREATE TABLE events (id int PRIMARY KEY, a int) PARTITION BY RANGE(id);
CREATE TABLE events_mid PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE(id);
ALTER TABLE events_mid ADD CONSTRAINT mid_local UNIQUE(id,a);
CREATE TABLE events_leaf PARTITION OF events_mid FOR VALUES FROM (0) TO (5);
ALTER INDEX events_mid_pkey RENAME TO mid_custom;
ALTER INDEX events_leaf_pkey RENAME TO leaf_custom;
ALTER INDEX events_leaf_id_a_key RENAME TO leaf_local_custom;`},
		{"quoted_identifiers", `s.'\$$.$pgschema_restore_0$"`, `CREATE TABLE "t.'\$$.$pgschema_restore_0$" (id int PRIMARY KEY, a int, UNIQUE(id,a)) PARTITION BY RANGE(id);
CREATE TABLE "leaf.'\$$.$pgschema_restore_0$" PARTITION OF "t.'\$$.$pgschema_restore_0$" FOR VALUES FROM (0) TO (10);
ALTER INDEX "leaf.'\$$.$pgschema_restore_0$_pkey" RENAME TO "idx.'\$$.$pgschema_restore_0$";
ALTER INDEX "leaf.'\$$.$pgschema_restore_0$_id_a_key" RENAME TO "a_very_long_destination_index_name_with_apostrophe_'_slash_\_end";`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			reset := func(db *sql.DB) {
				t.Helper()
				_, err := db.Exec("DROP SCHEMA IF EXISTS " + ir.QuoteIdentifier(fixture.schema) + " CASCADE; CREATE SCHEMA " + ir.QuoteIdentifier(fixture.schema) + "; SET search_path TO " + ir.QuoteIdentifier(fixture.schema) + ";")
				if err != nil {
					t.Fatal(err)
				}
			}
			reset(sourceDB)
			if _, err := sourceDB.Exec(fixture.sql); err != nil {
				t.Fatal(err)
			}
			want := inheritedIndexCatalog(t, sourceDB, fixture.schema)
			for _, multi := range []bool{false, true} {
				for _, qualified := range []bool{false, true} {
					t.Run(fmt.Sprintf("multi=%v/qualified=%v", multi, qualified), func(t *testing.T) {
						path := filepath.Join(t.TempDir(), "schema.sql")
						output, err := dump.ExecuteDump(&dump.DumpConfig{Host: sh, Port: sp, DB: sd, User: su, Password: sw, Schema: fixture.schema, SSLMode: "disable", File: path, MultiFile: multi, QualifySchema: qualified})
						if err != nil {
							t.Fatal(err)
						}
						if !multi {
							if err := os.WriteFile(path, []byte(output), 0600); err != nil {
								t.Fatal(err)
							}
						}
						text, err := include.NewProcessor(filepath.Dir(path)).ProcessFile(path)
						if err != nil {
							t.Fatal(err)
						}
						if !strings.Contains(text, "INTO STRICT") {
							t.Fatal("dump omitted restoration")
						}
						sourcePlan, err := planCmd.GeneratePlan(&planCmd.PlanConfig{Host: sh, Port: sp, DB: sd, User: su, Password: sw, Schema: fixture.schema, SSLMode: "disable", File: path}, sharedEmbeddedPG)
						if err != nil {
							t.Fatal(err)
						}
						if len(sourcePlan.Groups) != 0 {
							t.Fatalf("dump-to-source plan is not empty: %+v", sourcePlan.Groups)
						}
						if qualified && !multi {
							external, err := postgres.NewExternalDatabase(&postgres.ExternalDatabaseConfig{Host: sh, Port: sp, Database: sd, Username: su, Password: sw, SSLMode: "disable", TargetMajorVersion: majorVersion})
							if err != nil {
								t.Fatal(err)
							}
							defer external.Stop()
							externalPlan, err := planCmd.GeneratePlan(&planCmd.PlanConfig{Host: sh, Port: sp, DB: sd, User: su, Password: sw, Schema: fixture.schema, SSLMode: "disable", File: path}, external)
							if err != nil || len(externalPlan.Groups) != 0 {
								t.Fatalf("external dump-to-source plan: %+v, %v", externalPlan, err)
							}
						}
						for _, executor := range []string{"native", "sql"} {
							t.Run(executor, func(t *testing.T) {
								reset(targetDB)
								config := &planCmd.PlanConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: fixture.schema, SSLMode: "disable", File: path}
								migration, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
								if err != nil {
									t.Fatal(err)
								}
								if executor == "native" {
									if len(migration.Groups) != 1 {
										t.Fatalf("expected single creation group, got %d", len(migration.Groups))
									}
									// Inject both strict-lookup and destination-name failures into the real
									// creation group. Apply must propagate each failure and roll back tables.
									for _, failure := range []string{"lookup", "occupied"} {
										injected := false
										for i := range migration.Groups[0].Steps {
											step := &migration.Groups[0].Steps[i]
											if !strings.Contains(step.SQL, "INTO STRICT") {
												continue
											}
											original := step.SQL
											if failure == "lookup" {
												step.SQL = strings.Replace(original, "WHERE c.conrelid =", "WHERE false AND c.conrelid =", 1)
											} else {
												// The parent table is a relation in the same namespace as the leaf.
												parentName := "events"
												if fixture.name == "quoted_identifiers" {
													parentName = `t.'\$$.$pgschema_restore_0$`
												}
												literal := "E'" + strings.ReplaceAll(strings.ReplaceAll(parentName, `\`, `\\`), "'", "''") + "'"
												step.SQL = strings.Replace(original, "desired_names[position]);", literal+");", 1)
											}
											err = apply.ApplyMigration(&apply.ApplyConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: fixture.schema, SSLMode: "disable", Plan: migration, AutoApprove: true, NoColor: true, Quiet: true}, nil)
											step.SQL = original
											expected := "P0002"
											if failure == "occupied" {
												expected = "42P07"
											}
											var pgErr *pgconn.PgError
											if !errors.As(err, &pgErr) || pgErr.Code != expected {
												t.Fatalf("%s failure did not propagate %s: %v", failure, expected, err)
											}
											var count int
											if err := targetDB.QueryRow(`SELECT count(*) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1`, fixture.schema).Scan(&count); err != nil {
												t.Fatal(err)
											}
											if count != 0 {
												t.Fatalf("failed group left %d relations", count)
											}
											injected = true
											break
										}
										if !injected {
											t.Fatal("no restoration step for failure injection")
										}
									}
									err = apply.ApplyMigration(&apply.ApplyConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: fixture.schema, SSLMode: "disable", Plan: migration, AutoApprove: true, NoColor: true, Quiet: true}, nil)
								} else {
									_, err = targetDB.Exec(text)
								}
								if err != nil {
									t.Fatalf("%s replay failed: %v", executor, err)
								}
								if got := inheritedIndexCatalog(t, targetDB, fixture.schema); got != want {
									t.Fatalf("catalog mismatch\nwant: %s\ngot: %s", want, got)
								}
								followUp, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
								if err != nil {
									t.Fatal(err)
								}
								if len(followUp.Groups) != 0 {
									t.Fatalf("follow-up plan is not empty: %+v", followUp.Groups)
								}
							})
						}
					})
				}
			}
		})
	}
}

func inheritedIndexCatalog(t *testing.T, db *sql.DB, schema string) string {
	t.Helper()
	var catalog string
	err := db.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY table_name,index_name),'[]'::jsonb)::text FROM (
 SELECT t.relname AS table_name,i.relname AS index_name,pg_catalog.pg_get_indexdef(i.oid) AS definition,
        idx.indisvalid,idx.indisready,idx.indislive,c.conname,c.contype,
        pc.conname AS parent_constraint,pt.relname AS parent_table,
        ARRAY(SELECT p.relname FROM pg_catalog.pg_inherits h JOIN pg_catalog.pg_class p ON p.oid=h.inhparent WHERE h.inhrelid=i.oid ORDER BY h.inhseqno) AS parent_indexes,
        ARRAY(SELECT dep.deptype::text FROM pg_catalog.pg_depend dep WHERE dep.classid='pg_catalog.pg_class'::pg_catalog.regclass AND dep.objid=i.oid AND dep.refclassid='pg_catalog.pg_constraint'::pg_catalog.regclass AND dep.refobjid=c.oid ORDER BY dep.deptype) AS ownership
 FROM pg_catalog.pg_index idx JOIN pg_catalog.pg_class i ON i.oid=idx.indexrelid
 JOIN pg_catalog.pg_class t ON t.oid=idx.indrelid JOIN pg_catalog.pg_namespace n ON n.oid=t.relnamespace
 LEFT JOIN pg_catalog.pg_constraint c ON c.conindid=i.oid AND c.conrelid=t.oid AND c.contype IN ('p','u')
 LEFT JOIN pg_catalog.pg_constraint pc ON pc.oid=c.conparentid LEFT JOIN pg_catalog.pg_class pt ON pt.oid=pc.conrelid
 WHERE n.nspname=$1) x`, schema).Scan(&catalog)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
