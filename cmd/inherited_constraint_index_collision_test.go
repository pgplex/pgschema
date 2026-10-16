package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/cmd/apply"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/testutil"
)

func TestInheritedConstraintIndexTemporaryNameCollisions(t *testing.T) {
	if testing.Short() {
		t.Skip("embedded PostgreSQL integration")
	}
	t.Chdir(t.TempDir())
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, h, p, d, u, w := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	// The child needs a new function, so this family crosses creation batches
	// and deliberately exercises strict restoration.
	const desired = `CREATE TABLE events (id int PRIMARY KEY, a int) PARTITION BY RANGE(id);
CREATE FUNCTION event_value() RETURNS int LANGUAGE SQL AS 'SELECT a FROM events GROUP BY id';
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);
ALTER TABLE events_leaf ALTER a SET DEFAULT event_value();
ALTER INDEX events_leaf_pkey RENAME TO custom_leaf;`
	path := filepath.Join(t.TempDir(), "schema.sql")
	if err := os.WriteFile(path, []byte(desired), 0600); err != nil {
		t.Fatal(err)
	}
	config := &planCmd.PlanConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: "public", SSLMode: "disable", File: path}
	migration, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	if err != nil {
		t.Fatal(err)
	}
	injected := false
	for g := range migration.Groups {
		for s := range migration.Groups[g].Steps {
			step := &migration.Groups[g].Steps[s]
			if !strings.Contains(step.SQL, "INTO STRICT") {
				continue
			}
			setup := `    EXECUTE pg_catalog.format('CREATE TABLE %I (id int)', 'pgschema_inherited_' || index_ids[1] || '_0');
    EXECUTE pg_catalog.format('ALTER TABLE events_leaf ADD CONSTRAINT %I CHECK (id > -1)', 'pgschema_inherited_' || index_ids[1] || '_1');
    desired_names := pg_catalog.array_append(desired_names, 'pgschema_inherited_' || index_ids[1] || '_2');
`
			step.SQL = strings.Replace(step.SQL, "    FOR position IN", setup+"    FOR position IN", 1)
			cleanup := `    EXECUTE pg_catalog.format('DROP TABLE %I', 'pgschema_inherited_' || index_ids[1] || '_0');
    EXECUTE pg_catalog.format('ALTER TABLE events_leaf DROP CONSTRAINT %I', 'pgschema_inherited_' || index_ids[1] || '_1');
`
			step.SQL = strings.Replace(step.SQL, "END\n", cleanup+"END\n", 1)
			// Each candidate is occupied by a relation, a table constraint, or a
			// reserved destination. The fourth candidate must be the temporary name.
			step.SQL = strings.Replace(step.SQL, "            EXECUTE pg_catalog.format('ALTER INDEX", `            IF suffix <> 3 THEN
                RAISE EXCEPTION 'expected collision-free suffix 3, got %', suffix;
            END IF;
            EXECUTE pg_catalog.format('ALTER INDEX`, 1)
			injected = true
		}
	}
	if !injected {
		t.Fatal("missing restoration")
	}
	err = apply.ApplyMigration(&apply.ApplyConfig{Host: h, Port: p, DB: d, User: u, Password: w, Schema: "public", SSLMode: "disable", Plan: migration, AutoApprove: true, NoColor: true, Quiet: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow(`SELECT c.conname FROM pg_catalog.pg_constraint c WHERE c.conrelid='events_leaf'::pg_catalog.regclass AND c.contype='p'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "custom_leaf" {
		t.Fatalf("constraint ownership/name changed: %s", name)
	}
	followUp, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	if err != nil || len(followUp.Groups) != 0 {
		t.Fatalf("collision follow-up: %+v, %v", followUp, err)
	}
}
