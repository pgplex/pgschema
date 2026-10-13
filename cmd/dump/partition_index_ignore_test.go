package dump

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/include"
	"github.com/pgplex/pgschema/testutil"
)

func TestDumpCommand_IgnoredPartitionIndexes(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	source := testutil.SetupPostgres(t)
	defer source.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, source)
	defer conn.Close()

	const setup = `
CREATE TABLE events (bucket int, id int) PARTITION BY RANGE (bucket);
CREATE TABLE events_early PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (id);
CREATE TABLE events_late PARTITION OF events FOR VALUES FROM (10) TO (20);
CREATE TABLE events_first PARTITION OF events_early FOR VALUES FROM (0) TO (10);
CREATE TABLE events_second PARTITION OF events_early FOR VALUES FROM (10) TO (20);
CREATE UNIQUE INDEX event_unique ON ONLY events (bucket, id);
CREATE UNIQUE INDEX early_custom_unique ON ONLY events_early (bucket, id);
CREATE UNIQUE INDEX late_custom_unique ON events_late (bucket, id);
CREATE UNIQUE INDEX first_custom_unique ON events_first (bucket, id);
CREATE UNIQUE INDEX second_custom_unique ON events_second (bucket, id);
ALTER INDEX early_custom_unique ATTACH PARTITION first_custom_unique;
ALTER INDEX early_custom_unique ATTACH PARTITION second_custom_unique;
ALTER INDEX event_unique ATTACH PARTITION early_custom_unique;
ALTER INDEX event_unique ATTACH PARTITION late_custom_unique;
CREATE TABLE event_links (bucket int, id int, FOREIGN KEY (bucket, id) REFERENCES events (bucket, id));
CREATE INDEX unattached_lookup ON events_first (id);
CREATE INDEX partial_lookup ON ONLY events (id);
CREATE INDEX late_partial_lookup ON events_late (id);
ALTER INDEX partial_lookup ATTACH PARTITION late_partial_lookup;
`
	for _, tc := range []struct {
		name            string
		ignore          string
		parent          string
		child           string
		dropFK          bool
		absent          string
		constraintChild bool
	}{
		{name: "complete_family"},
		{name: "constraint_owned_leaf", constraintChild: true},
		{name: "ignored_constraint", constraintChild: true, ignore: "[constraints]\npatterns = [\"late_custom_unique\"]\n", parent: "event_unique", child: "late_custom_unique"},
		{name: "ignored_constraint_table", constraintChild: true, ignore: "[tables]\npatterns = [\"events_late\"]\n", parent: "event_unique", child: "late_custom_unique"},
		{name: "constraint_owned_index_pattern", constraintChild: true, ignore: "[indexes]\npatterns = [\"late_custom_unique\"]\n"},
		{name: "ignored_leaf", ignore: "[indexes]\npatterns = [\"late_custom_unique\"]\n", parent: "event_unique", child: "late_custom_unique"},
		{name: "ignored_intermediate", ignore: "[indexes]\npatterns = [\"early_custom_unique\"]\n", parent: "event_unique", child: "early_custom_unique"},
		{name: "ignored_nested_leaf", ignore: "[indexes]\npatterns = [\"first_custom_unique\"]\n", parent: "early_custom_unique", child: "first_custom_unique"},
		{name: "ignored_partition_table", ignore: "[tables]\npatterns = [\"events_first\"]\n", parent: "early_custom_unique", child: "first_custom_unique"},
		{name: "ignored_leaf_without_fk", ignore: "[indexes]\npatterns = [\"late_custom_unique\"]\n", parent: "event_unique", child: "late_custom_unique", dropFK: true},
		{name: "ignored_child_of_invalid_parent", ignore: "[indexes]\npatterns = [\"late_partial_lookup\"]\n", parent: "partial_lookup", child: "late_partial_lookup"},
		{name: "ignored_unattached", ignore: "[indexes]\npatterns = [\"unattached_lookup\"]\n", absent: "unattached_lookup"},
		{name: "ignored_family", ignore: "[indexes]\npatterns = [\"event_unique\", \"*custom_unique\"]\n[constraints]\npatterns = [\"event_links_bucket_id_fkey\"]\n", absent: "event_unique"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.WriteFile(".pgschemaignore", []byte(tc.ignore), 0600); err != nil {
				t.Fatal(err)
			}
			for _, multiFile := range []bool{false, true} {
				for _, existingOutput := range []bool{false, true} {
					t.Run(fmt.Sprintf("multi=%v/existing=%v", multiFile, existingOutput), func(t *testing.T) {
						fixture := setup
						if tc.constraintChild {
							fixture = strings.Replace(fixture, "CREATE UNIQUE INDEX late_custom_unique ON events_late (bucket, id);", "ALTER TABLE events_late ADD CONSTRAINT late_custom_unique UNIQUE (bucket, id);", 1)
						}
						if _, err := conn.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;` + fixture); err != nil {
							t.Fatal(err)
						}
						if tc.dropFK {
							if _, err := conn.Exec(`ALTER TABLE event_links DROP CONSTRAINT event_links_bucket_id_fkey`); err != nil {
								t.Fatal(err)
							}
						}
						outputDir := t.TempDir()
						path := filepath.Join(outputDir, "schema.sql")
						const original = "-- existing output must survive a failed dump\n"
						if existingOutput {
							if err := os.WriteFile(path, []byte(original), 0600); err != nil {
								t.Fatal(err)
							}
						}
						output, err := ExecuteDump(&DumpConfig{
							Host: host, Port: port, DB: dbname, User: user, Password: password,
							Schema: "public", SSLMode: "disable", MultiFile: multiFile, File: path,
						})
						if tc.child != "" {
							want := "cannot dump index public." + tc.parent + ": attached index public." + tc.child + " is excluded"
							if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), ".pgschemaignore") {
								t.Fatalf("Expected actionable dependency error %q, got %v", want, err)
							}
							if output != "" {
								t.Fatal("Failed dump returned partial SQL")
							}
							entries, readErr := os.ReadDir(outputDir)
							if readErr != nil {
								t.Fatal(readErr)
							}
							wantFiles := 0
							if existingOutput {
								wantFiles = 1
								content, readErr := os.ReadFile(path)
								if readErr != nil || string(content) != original {
									t.Fatalf("Failed dump changed existing output: %q, %v", content, readErr)
								}
							}
							if len(entries) != wantFiles {
								t.Fatalf("Failed dump created files: %v", entries)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						if multiFile {
							output, err = include.NewProcessor(outputDir).ProcessFile(path)
							if err != nil {
								t.Fatal(err)
							}
						}
						if tc.absent != "" && strings.Contains(output, tc.absent) {
							t.Fatalf("Dump contains ignored index %s", tc.absent)
						}
						if _, err := conn.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;` + output); err != nil {
							t.Fatalf("Allowed dump failed replay: %v", err)
						}
					})
				}
			}
		})
	}
}

// The ignored leaf has only an inherited constraint index. No standalone leaf
// index can cause an unrelated dependency error and mask this regression.
func TestDumpCommand_IgnoredConstraintIndexDescendant(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".pgschemaignore", []byte("[tables]\npatterns = [\"events_leaf\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := testutil.SetupPostgres(t)
	defer source.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, source)
	defer conn.Close()
	const setup = `
CREATE TABLE events (bucket int, id int) PARTITION BY RANGE (bucket);
CREATE TABLE events_mid PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (id);
CREATE TABLE events_branch PARTITION OF events_mid FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (bucket);
CREATE TABLE events_leaf PARTITION OF events_branch FOR VALUES FROM (0) TO (10);
ALTER TABLE events_mid ADD CONSTRAINT mid_unique UNIQUE (bucket, id);
CREATE UNIQUE INDEX event_unique ON ONLY events (bucket, id);
ALTER INDEX event_unique ATTACH PARTITION mid_unique;
CREATE TABLE event_links (bucket int, id int, FOREIGN KEY (bucket, id) REFERENCES events (bucket, id));
`
	if _, err := conn.Exec(setup); err != nil {
		t.Fatal(err)
	}
	for _, multiFile := range []bool{false, true} {
		for _, existingOutput := range []bool{false, true} {
			t.Run(fmt.Sprintf("multi=%v/existing=%v", multiFile, existingOutput), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "schema.sql")
				const original = "-- preserve existing output\n"
				if existingOutput {
					if err := os.WriteFile(path, []byte(original), 0600); err != nil {
						t.Fatal(err)
					}
				}
				output, err := ExecuteDump(&DumpConfig{
					Host: host, Port: port, DB: dbname, User: user, Password: password,
					Schema: "public", SSLMode: "disable", MultiFile: multiFile, File: path,
				})
				if err == nil || !strings.Contains(err.Error(), "event_unique") || !strings.Contains(err.Error(), "mid_unique") || !strings.Contains(err.Error(), "events_leaf") || !strings.Contains(err.Error(), ".pgschemaignore") {
					t.Fatalf("Expected actionable nested dependency error, got %v", err)
				}
				if output != "" {
					t.Fatal("Failed dump returned partial SQL")
				}
				entries, err := os.ReadDir(dir)
				if err != nil {
					t.Fatal(err)
				}
				wantFiles := 0
				if existingOutput {
					wantFiles = 1
					content, err := os.ReadFile(path)
					if err != nil || string(content) != original {
						t.Fatalf("Failed dump changed existing output: %q, %v", content, err)
					}
				}
				if len(entries) != wantFiles {
					t.Fatalf("Failed dump created files: %v", entries)
				}
			})
		}
	}
}
