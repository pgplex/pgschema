package dump

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/internal/include"
	"github.com/pgplex/pgschema/testutil"
)

// TestDumpCommand_PartitionIndexIdentity compares each dump with an independently
// created source, including deliberately unattached equivalent indexes.
func TestDumpCommand_PartitionIndexIdentity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	source := testutil.SetupPostgres(t)
	defer source.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, source)
	defer conn.Close()
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	ctx := context.Background()

	const setup = `
CREATE TABLE events (bucket int NOT NULL, id int NOT NULL, label text, payload text) PARTITION BY RANGE (bucket);
CREATE TABLE events_early PARTITION OF events FOR VALUES FROM (0) TO (10);
CREATE TABLE events_late PARTITION OF events FOR VALUES FROM (10) TO (20);
CREATE INDEX event_lookup ON ONLY events (label text_pattern_ops DESC NULLS LAST, id) INCLUDE (payload);
CREATE INDEX early_custom_lookup ON events_early (label text_pattern_ops DESC NULLS LAST, id) INCLUDE (payload);
CREATE INDEX late_custom_lookup ON events_late (label text_pattern_ops DESC NULLS LAST, id) INCLUDE (payload);
ALTER INDEX event_lookup ATTACH PARTITION early_custom_lookup;
ALTER INDEX event_lookup ATTACH PARTITION late_custom_lookup;
CREATE INDEX early_unattached_lookup ON events_early (label text_pattern_ops DESC NULLS LAST, id) INCLUDE (payload);
CREATE UNIQUE INDEX event_unique ON ONLY events (bucket, id);
ALTER TABLE events_early ADD CONSTRAINT early_custom_unique UNIQUE (bucket, id);
CREATE UNIQUE INDEX late_custom_unique ON events_late (bucket, id);
ALTER INDEX event_unique ATTACH PARTITION early_custom_unique;
ALTER INDEX event_unique ATTACH PARTITION late_custom_unique;
CREATE TABLE event_links (bucket int, id int, FOREIGN KEY (bucket, id) REFERENCES events (bucket, id));
-- This parent intentionally remains invalid because only one child is attached.
CREATE INDEX event_partial ON ONLY events ((lower(label))) WHERE id > 0;
CREATE INDEX early_custom_partial ON events_early ((lower(label))) WHERE id > 0;
CREATE INDEX late_unattached_partial ON events_late ((lower(label))) WHERE id > 0;
ALTER INDEX event_partial ATTACH PARTITION early_custom_partial;
CREATE INDEX event_unattached_parent ON ONLY events (id);
CREATE INDEX early_unattached_id ON events_early (id);
CREATE INDEX "Event.Lookup" ON ONLY events (id DESC);
CREATE INDEX "Early.Lookup" ON events_early (id DESC);
CREATE INDEX "Late.Lookup" ON events_late (id DESC);
ALTER INDEX "Event.Lookup" ATTACH PARTITION "Early.Lookup";
ALTER INDEX "Event.Lookup" ATTACH PARTITION "Late.Lookup";
CREATE TABLE empty_events (id int) PARTITION BY RANGE (id);
CREATE INDEX empty_lookup ON empty_events (id);
-- The root index sorts before its partitioned child to exercise recursive ordering.
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
ALTER TABLE archive_events_early ADD CONSTRAINT archive_local_unique UNIQUE (bucket, label);
CREATE UNIQUE INDEX archive_unique ON ONLY archive_events (bucket, label);
ALTER INDEX archive_unique ATTACH PARTITION archive_local_unique;
CREATE TABLE archive_links (bucket int, label text, FOREIGN KEY (bucket, label) REFERENCES archive_events (bucket, label));
COMMENT ON INDEX event_lookup IS 'Parent lookup';
COMMENT ON INDEX early_custom_lookup IS 'Named child lookup';
-- Inherited constraint indexes keep their catalog names even when a standalone
-- index already holds the generated name (issue #653).
CREATE TABLE keyed_events (bucket int NOT NULL, id int NOT NULL, note text,
    PRIMARY KEY (bucket, id), CONSTRAINT keyed_events_note_key UNIQUE (bucket, id, note)) PARTITION BY RANGE (bucket);
CREATE INDEX keyed_events_early_pkey ON keyed_events (id);
CREATE TABLE keyed_events_early PARTITION OF keyed_events FOR VALUES FROM (0) TO (10);
CREATE TABLE keyed_events_late PARTITION OF keyed_events FOR VALUES FROM (10) TO (20) PARTITION BY RANGE (bucket);
CREATE TABLE keyed_events_late_a PARTITION OF keyed_events_late FOR VALUES FROM (10) TO (15);
ALTER INDEX keyed_events_late_a_pkey RENAME TO keyed_late_a_pk;
CREATE INDEX keyed_events_late_a_pkey ON keyed_events_late_a (note);
CREATE TABLE keyed_links (bucket int, id int, FOREIGN KEY (bucket, id) REFERENCES keyed_events (bucket, id));
`
	for _, constraintType := range []string{"UNIQUE", "PRIMARY KEY"} {
		for _, multiFile := range []bool{false, true} {
			for _, noComments := range []bool{false, true} {
				for _, qualifySchema := range []bool{false, true} {
					t.Run(fmt.Sprintf("constraint=%s/multi=%v/noComments=%v/qualified=%v", constraintType, multiFile, noComments, qualifySchema), func(t *testing.T) {
						fixture := strings.Replace(setup, "early_custom_unique UNIQUE", "early_custom_unique "+constraintType, 1)
						if _, err := conn.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`+fixture); err != nil {
							t.Fatal(err)
						}
						want := readIndexCatalog(t, conn)
						wantFKs := foreignKeyCatalog(t, conn)
						path := filepath.Join(t.TempDir(), "schema.sql")
						config := &DumpConfig{
							Host: host, Port: port, DB: dbname, User: user, Password: password,
							Schema: "public", SSLMode: "disable", MultiFile: multiFile, File: path,
							NoComments: noComments, QualifySchema: qualifySchema,
						}
						output, err := ExecuteDump(config)
						if err != nil {
							t.Fatal(err)
						}
						if !multiFile {
							if err := os.WriteFile(path, []byte(output), 0600); err != nil {
								t.Fatal(err)
							}
						}
						// Compare the dump with the independent source BEFORE replay changes it.
						migration, err := plan.GeneratePlan(&plan.PlanConfig{
							Host: host, Port: port, DB: dbname, User: user, Password: password,
							Schema: "public", SSLMode: "disable", File: path,
						}, provider)
						if err != nil {
							t.Errorf("Plan against independent source failed: %v", err)
						} else if len(migration.Groups) != 0 {
							t.Errorf("Expected empty plan against independent source, got: %+v", migration.Groups)
						}
						sqlText, err := include.NewProcessor(filepath.Dir(path)).ProcessFile(path)
						if err != nil {
							t.Fatal(err)
						}
						prefix := ""
						if qualifySchema {
							prefix = "public."
						}
						attachment := "ALTER INDEX " + prefix + "event_lookup ATTACH PARTITION " + prefix + "early_custom_lookup;"
						if !strings.Contains(sqlText, attachment) {
							t.Errorf("Missing explicit attachment: %s", attachment)
						}
						if strings.Contains(sqlText, "-- Name:") == noComments {
							t.Error("Unexpected object header formatting")
						}
						mainContent, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						repeated, err := ExecuteDump(config)
						if err != nil {
							t.Fatal(err)
						}
						if multiFile {
							repeatedMain, err := os.ReadFile(path)
							if err != nil {
								t.Fatal(err)
							}
							if string(repeatedMain) != string(mainContent) {
								t.Error("Non-deterministic include order or filenames")
							}
							repeated, err = include.NewProcessor(filepath.Dir(path)).ProcessFile(path)
							if err != nil {
								t.Fatal(err)
							}
						}
						if repeated != sqlText {
							t.Error("Non-deterministic SQL output")
						}
						if _, err := conn.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`+sqlText); err != nil {
							t.Fatalf("Dump replay failed: %v", err)
						}
						if got := readIndexCatalog(t, conn); got != want {
							t.Errorf("Index catalog changed after replay:\ngot: %s\nwant: %s", got, want)
						}
						if got := foreignKeyCatalog(t, conn); !reflect.DeepEqual(got, wantFKs) {
							t.Errorf("Foreign keys changed after replay: got %v, want %v", got, wantFKs)
						}
					})
				}
			}
		}
	}

}

func readIndexCatalog(t *testing.T, conn *sql.DB) string {
	t.Helper()
	var catalog string
	err := conn.QueryRowContext(context.Background(), `
SELECT jsonb_agg(to_jsonb(x) ORDER BY table_name, name)::text FROM (
    SELECT t.relname AS table_name, c.relname AS name, c.relkind AS kind,
           pg_get_indexdef(c.oid) AS definition, i.indisvalid AS valid,
           i.indisready AS ready, i.indislive AS live,
           obj_description(c.oid, 'pg_class') AS comment,
           ARRAY(SELECT p.relname FROM pg_inherits h JOIN pg_class p ON p.oid = h.inhparent
                 WHERE h.inhrelid = c.oid ORDER BY h.inhseqno) AS parents
    FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
    JOIN pg_class t ON t.oid = i.indrelid JOIN pg_namespace n ON n.oid = t.relnamespace
    WHERE n.nspname = 'public'
) x`).Scan(&catalog)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}
