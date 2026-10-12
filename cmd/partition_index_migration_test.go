package cmd

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/apply"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/testutil"
)

// TestPlanAndApplyPartitionIndexIdentity compares native migration and direct SQL
// replay with an independently created source, rather than a previous migration.
func TestPlanAndApplyPartitionIndexIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	t.Chdir(t.TempDir())
	source := testutil.SetupPostgres(t)
	defer source.Stop()
	sourceConn, _, _, _, _, _ := testutil.ConnectToPostgres(t, source)
	defer sourceConn.Close()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	targetConn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, target)
	defer targetConn.Close()

	const desired = `
CREATE TABLE events (bucket int, id int, label text) PARTITION BY RANGE (bucket);
CREATE TABLE events_early PARTITION OF events FOR VALUES FROM (0) TO (10);
CREATE TABLE events_late PARTITION OF events FOR VALUES FROM (10) TO (20);
CREATE INDEX event_lookup ON ONLY events (label);
CREATE INDEX early_custom_lookup ON events_early (label);
CREATE INDEX late_custom_lookup ON events_late (label);
ALTER INDEX event_lookup ATTACH PARTITION early_custom_lookup;
ALTER INDEX event_lookup ATTACH PARTITION late_custom_lookup;
CREATE INDEX early_unattached_lookup ON events_early (label);
CREATE UNIQUE INDEX event_unique ON ONLY events (bucket, id);
CREATE UNIQUE INDEX early_custom_unique ON events_early (bucket, id);
CREATE UNIQUE INDEX late_custom_unique ON events_late (bucket, id);
ALTER INDEX event_unique ATTACH PARTITION early_custom_unique;
ALTER INDEX event_unique ATTACH PARTITION late_custom_unique;
CREATE TABLE event_links (bucket int, id int, FOREIGN KEY (bucket, id) REFERENCES events (bucket, id));
CREATE INDEX event_partial ON ONLY events ((lower(label))) WHERE id > 0;
CREATE INDEX early_custom_partial ON events_early ((lower(label))) WHERE id > 0;
CREATE INDEX late_unattached_partial ON events_late ((lower(label))) WHERE id > 0;
ALTER INDEX event_partial ATTACH PARTITION early_custom_partial;
CREATE TABLE archive_events (bucket int, label text) PARTITION BY RANGE (bucket);
CREATE TABLE archive_events_early PARTITION OF archive_events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (bucket);
CREATE TABLE archive_events_first PARTITION OF archive_events_early FOR VALUES FROM (0) TO (5);
CREATE TABLE archive_events_second PARTITION OF archive_events_early FOR VALUES FROM (5) TO (10);
CREATE INDEX a_archive_lookup ON ONLY archive_events (label);
CREATE INDEX z_archive_lookup ON ONLY archive_events_early (label);
CREATE INDEX first_custom_lookup ON archive_events_first (label);
CREATE INDEX second_custom_lookup ON archive_events_second (label);
ALTER INDEX z_archive_lookup ATTACH PARTITION first_custom_lookup;
ALTER INDEX z_archive_lookup ATTACH PARTITION second_custom_lookup;
ALTER INDEX a_archive_lookup ATTACH PARTITION z_archive_lookup;
CREATE INDEX first_unattached_lookup ON archive_events_first (label);
CREATE INDEX "Event.Lookup" ON ONLY events (id DESC);
CREATE INDEX "Early.Lookup" ON events_early (id DESC);
CREATE INDEX "Late.Lookup" ON events_late (id DESC);
ALTER INDEX "Event.Lookup" ATTACH PARTITION "Early.Lookup";
ALTER INDEX "Event.Lookup" ATTACH PARTITION "Late.Lookup";
CREATE TABLE empty_events (id int) PARTITION BY RANGE (id);
CREATE INDEX empty_lookup ON empty_events (id);
COMMENT ON INDEX event_lookup IS 'Parent lookup';
COMMENT ON INDEX early_custom_lookup IS 'Named child lookup';
`
	for _, initial := range []struct{ name, sql, desiredPrefix string }{
		{name: "empty"},
		{name: "unrelated_table", sql: "CREATE TABLE unrelated (id integer);\n", desiredPrefix: "CREATE TABLE unrelated (id integer);\n"},
		{name: "existing_referencing_table", sql: "CREATE TABLE event_links (bucket int, id int);\n"},
	} {
		for _, executor := range []string{"native", "sql"} {
			t.Run(initial.name+"/"+executor, func(t *testing.T) {
				if _, err := sourceConn.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;` + initial.desiredPrefix + desired); err != nil {
					t.Fatal(err)
				}
				wantCount, wantCatalog := migrationIndexCatalog(t, sourceConn)
				if wantCount != 19 {
					t.Fatalf("Expected 19 source indexes, got %d", wantCount)
				}
				if _, err := targetConn.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;` + initial.sql); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "schema.sql")
				if err := os.WriteFile(path, []byte(initial.desiredPrefix+desired), 0600); err != nil {
					t.Fatal(err)
				}
				config := &planCmd.PlanConfig{
					Host: host, Port: port, DB: dbname, User: user, Password: password,
					Schema: "public", SSLMode: "disable", File: path,
				}
				migration, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
				if err != nil {
					t.Fatal(err)
				}
				var statements []string
				for _, group := range migration.Groups {
					for _, step := range group.Steps {
						if step.SQL != "" {
							statements = append(statements, step.SQL)
						}
					}
				}
				generatedSQL := strings.Join(statements, "\n")
				if !strings.Contains(generatedSQL, "event_lookup ON ONLY events") ||
					!strings.Contains(generatedSQL, "ALTER INDEX event_lookup ATTACH PARTITION early_custom_lookup") {
					t.Errorf("Generated migration does not preserve explicit index attachments:\n%s", generatedSQL)
				}
				if strings.Contains(generatedSQL, "CONCURRENTLY") {
					t.Errorf("New-table index creation must remain synchronous:\n%s", generatedSQL)
				}
				if initial.name == "empty" && executor == "native" {
					if len(migration.Groups) != 1 {
						t.Fatalf("Expected one transactional bootstrap group, got %d", len(migration.Groups))
					}
					// A failed attachment must propagate without a recursive fallback.
					// PostgreSQL must roll back the ordinary group before a retry.
					failedAttachment := false
					for i := range migration.Groups[0].Steps {
						step := &migration.Groups[0].Steps[i]
						if step.SQL != "ALTER INDEX event_lookup ATTACH PARTITION early_custom_lookup;" {
							continue
						}
						originalSQL := step.SQL
						step.SQL = "ALTER INDEX event_lookup ATTACH PARTITION missing_lookup;"
						applyErr := apply.ApplyMigration(&apply.ApplyConfig{
							Host: host, Port: port, DB: dbname, User: user, Password: password,
							Schema: "public", SSLMode: "disable", Plan: migration,
							AutoApprove: true, NoColor: true, Quiet: true,
						}, nil)
						step.SQL = originalSQL
						if applyErr == nil || !strings.Contains(applyErr.Error(), "missing_lookup") {
							t.Fatalf("Expected attachment failure, got %v", applyErr)
						}
						var tableCount int
						if err := targetConn.QueryRow(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = 'public' AND c.relkind IN ('r', 'p')`).Scan(&tableCount); err != nil {
							t.Fatal(err)
						}
						if count, _ := migrationIndexCatalog(t, targetConn); count != 0 || tableCount != 0 {
							t.Fatalf("Failed group left %d tables and %d indexes", tableCount, count)
						}
						failedAttachment = true
						break
					}
					if !failedAttachment {
						t.Fatal("No explicit attachment available for failure test")
					}
				}
				if executor == "native" {
					err = apply.ApplyMigration(&apply.ApplyConfig{
						Host: host, Port: port, DB: dbname, User: user, Password: password,
						Schema: "public", SSLMode: "disable", Plan: migration,
						AutoApprove: true, NoColor: true, Quiet: true,
					}, nil)
				} else {
					// Use the emitted SQL directly to isolate generation from the executor.
					for _, statement := range statements {
						if _, err = targetConn.Exec(statement); err != nil {
							break
						}
					}
				}
				if err != nil {
					t.Fatalf("Migration failed: %v", err)
				}
				gotCount, gotCatalog := migrationIndexCatalog(t, targetConn)
				if gotCatalog != wantCatalog {
					t.Errorf("Index catalog mismatch: expected %d indexes, got %d\nwant: %s\ngot: %s", wantCount, gotCount, wantCatalog, gotCatalog)
				}
				if got, want := migrationFKCatalog(t, targetConn), migrationFKCatalog(t, sourceConn); got != want {
					t.Errorf("FK catalog mismatch:\nwant: %s\ngot: %s", want, got)
				}
				followUp, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
				if err != nil {
					t.Fatal(err)
				}
				if len(followUp.Groups) != 0 {
					t.Errorf("Expected empty follow-up plan, got %+v", followUp.Groups)
				}

				// Keep incremental creation under an existing parent and unrelated
				// table alterations covered after the independent bootstrap check.
				const incremental = `ALTER TABLE event_links ADD COLUMN note text;
CREATE TABLE events_future PARTITION OF events FOR VALUES FROM (20) TO (30);`
				if _, err := sourceConn.Exec(incremental); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(initial.desiredPrefix+desired+incremental), 0600); err != nil {
					t.Fatal(err)
				}
				if err := apply.ApplyMigration(&apply.ApplyConfig{
					Host: host, Port: port, DB: dbname, User: user, Password: password,
					Schema: "public", SSLMode: "disable", File: path,
					AutoApprove: true, NoColor: true, Quiet: true,
				}, sharedEmbeddedPG); err != nil {
					t.Fatalf("Incremental migration failed: %v", err)
				}
				_, wantCatalog = migrationIndexCatalog(t, sourceConn)
				_, gotCatalog = migrationIndexCatalog(t, targetConn)
				if gotCatalog != wantCatalog {
					t.Errorf("Incremental migration changed index identity:\nwant: %s\ngot: %s", wantCatalog, gotCatalog)
				}
				if got, want := migrationFKCatalog(t, targetConn), migrationFKCatalog(t, sourceConn); got != want {
					t.Errorf("Incremental migration changed FKs:\nwant: %s\ngot: %s", want, got)
				}
				followUp, err = planCmd.GeneratePlan(config, sharedEmbeddedPG)
				if err != nil || len(followUp.Groups) != 0 {
					t.Fatalf("Incremental follow-up plan is not empty: %+v, %v", followUp, err)
				}
			})
		}
	}
}

func migrationIndexCatalog(t *testing.T, conn *sql.DB) (int, string) {
	t.Helper()
	var count int
	var catalog string
	err := conn.QueryRow(`
SELECT count(*), COALESCE(jsonb_agg(to_jsonb(x) ORDER BY name), '[]'::jsonb)::text
FROM (
    SELECT c.relname AS name, t.relname AS table_name,
           pg_get_indexdef(c.oid) AS definition, i.indisvalid AS valid,
           i.indisready AS ready, i.indislive AS live,
           obj_description(c.oid, 'pg_class') AS comment,
           ARRAY(SELECT pn.nspname || '.' || p.relname
                 FROM pg_inherits h JOIN pg_class p ON p.oid = h.inhparent
                 JOIN pg_namespace pn ON pn.oid = p.relnamespace
                 WHERE h.inhrelid = c.oid ORDER BY h.inhseqno) AS parents
    FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
    JOIN pg_class t ON t.oid = i.indrelid JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = 'public'
) x`).Scan(&count, &catalog)
	if err != nil {
		t.Fatal(err)
	}
	return count, catalog
}

// Compare declared FKs. PostgreSQL creates internal child constraints for a
// partitioned target; pgschema does not manage those constraints independently.
func migrationFKCatalog(t *testing.T, conn *sql.DB) string {
	t.Helper()
	var catalog string
	err := conn.QueryRow(`
SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY table_name, name), '[]'::jsonb)::text
FROM (
    SELECT c.conrelid::regclass::text AS table_name, c.conname AS name,
           pg_get_constraintdef(c.oid) AS definition
    FROM pg_constraint c JOIN pg_namespace n ON n.oid = c.connamespace
    WHERE n.nspname = 'public' AND c.contype = 'f' AND c.conparentid = 0
) x`).Scan(&catalog)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestPlanPartitionIndexFilteringCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, target)
	defer conn.Close()
	const desired = `
CREATE TABLE events (id int, label text) PARTITION BY RANGE (id);
CREATE TABLE events_early PARTITION OF events FOR VALUES FROM (0) TO (10);
CREATE TABLE events_late PARTITION OF events FOR VALUES FROM (10) TO (20);
CREATE INDEX parent_lookup ON ONLY events (label);
CREATE INDEX custom_lookup ON events_early (label);
ALTER INDEX parent_lookup ATTACH PARTITION custom_lookup;
`
	for _, tc := range []struct{ name, ignore string }{
		{name: "ignored_attached_index", ignore: "[indexes]\npatterns = [\"custom_lookup\"]\n"},
		{name: "ignored_attached_table", ignore: "[tables]\npatterns = [\"events_early\"]\n"},
		{name: "ignored_table_without_index", ignore: "[tables]\npatterns = [\"events_late\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile(".pgschemaignore", []byte(tc.ignore), 0600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "schema.sql")
			if err := os.WriteFile(path, []byte(desired), 0600); err != nil {
				t.Fatal(err)
			}
			migration, err := planCmd.GeneratePlan(&planCmd.PlanConfig{
				Host: host, Port: port, DB: dbname, User: user, Password: password,
				Schema: "public", SSLMode: "disable", File: path,
			}, sharedEmbeddedPG)
			if err != nil {
				t.Fatal(err)
			}
			var sqlText strings.Builder
			for _, group := range migration.Groups {
				for _, step := range group.Steps {
					sqlText.WriteString(step.SQL)
				}
			}
			text := sqlText.String()
			if !strings.Contains(text, "CREATE INDEX IF NOT EXISTS parent_lookup ON events (label);") || strings.Contains(text, "ATTACH PARTITION") {
				t.Fatalf("Filtered hierarchy must retain recursive migration behavior:\n%s", text)
			}
		})
	}
}
