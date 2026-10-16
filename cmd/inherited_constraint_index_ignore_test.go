package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/dump"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/testutil"
)

func TestIgnoredOriginInheritedConstraintIndexes(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded PostgreSQL integration")
	}
	t.Chdir(t.TempDir())
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, h, p, d, u, w := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	const fixture = `CREATE TABLE events (id int PRIMARY KEY) PARTITION BY RANGE(id);
CREATE TABLE events_mid PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE(id);
CREATE TABLE events_leaf PARTITION OF events_mid FOR VALUES FROM (0) TO (5);
ALTER INDEX events_mid_pkey RENAME TO custom_mid;
ALTER INDEX events_leaf_pkey RENAME TO custom_leaf;`
	if _, err := db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".pgschemaignore", []byte("[constraints]\npatterns = [\"events_pkey\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "schema.sql")
	text, err := dump.ExecuteDump(&dump.DumpConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: "public", SSLMode: "disable", File: path})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "INTO STRICT") || strings.Contains(text, "PRIMARY KEY") {
		t.Fatalf("ignored constraint family was restored:\n%s", text)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	config := &planCmd.PlanConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: "public", SSLMode: "disable", File: path}
	migration, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	if err != nil {
		t.Fatal(err)
	}
	if len(migration.Groups) != 0 {
		t.Fatalf("ignored-family dump-to-plan is not empty: %+v", migration.Groups)
	}
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;` + text); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pg_catalog.pg_constraint c JOIN pg_catalog.pg_namespace n ON n.oid=c.connamespace WHERE n.nspname='public' AND c.contype='p'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("replay created %d ignored primary keys", count)
	}
}
