package dump

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/testutil"
)

// TestDumpCommand_Issue580MultiFileIncludeOrder verifies that the \i include
// order written to main.sql by `dump --multi-file` respects cross-category
// dependencies (issue #580). A function whose signature uses a table's or
// view's row type must be included after that relation, while functions with
// no such dependency (trigger functions, policy helpers) must stay ahead of
// the tables that reference them.
//
// The multi-file output is replayed statement-file by statement-file into a
// fresh schema, which is the check that matters: main.sql must apply cleanly
// to an empty database.
func TestDumpCommand_Issue580MultiFileIncludeOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()

	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	ctx := context.Background()
	setup := `
-- Function with no relation dependency; a trigger below references it, so it
-- must be included before that table (the behavior that already worked).
CREATE FUNCTION touch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at := now(); RETURN NEW; END $$;

-- Function used by an RLS policy; must also stay ahead of its table.
CREATE FUNCTION current_owner() RETURNS text LANGUAGE sql AS $$ SELECT current_user::text $$;

-- Function used by a domain CHECK; the domain and the table using it follow.
CREATE FUNCTION is_ok(text) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$ SELECT $1 <> '' $$;
CREATE DOMAIN ok_text AS text CHECK (is_ok(VALUE));

CREATE TABLE audited (id int, updated_at timestamptz);
CREATE TRIGGER touch_trg BEFORE UPDATE ON audited FOR EACH ROW EXECUTE FUNCTION touch();

CREATE TABLE docs (id int, owner text);
ALTER TABLE docs ENABLE ROW LEVEL SECURITY;
CREATE POLICY docs_owner ON docs USING (owner = current_owner());

CREATE TABLE uses_domain (v ok_text);

-- plpgsql trigger function whose body writes to another table. Its body is
-- not validated at creation, so it must still be included before the table
-- that bundles its trigger, even though the body mentions audit_log.
CREATE TABLE audit_log (id int, note text);
CREATE FUNCTION log_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN INSERT INTO audit_log (note) VALUES ('changed'); RETURN NEW; END $$;
CREATE TABLE watched (id int);
CREATE TRIGGER watched_trg AFTER INSERT ON watched FOR EACH ROW EXECUTE FUNCTION log_change();

-- SQL-language function whose body queries a table that the diff defers
-- (its default calls a new function). PostgreSQL validates the SQL body at
-- creation, so the diff's order helper -> table -> function must be kept.
CREATE FUNCTION default_label() RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT 'x'::text $$;
CREATE TABLE labeled (id int, label text DEFAULT default_label());
CREATE FUNCTION count_labeled() RETURNS bigint LANGUAGE sql AS $$ SELECT count(*) FROM labeled $$;

-- Issue #580 variant A: function taking a table's row type as a parameter.
CREATE TABLE base_table (id int, label text);
CREATE FUNCTION use_table_row(r base_table) RETURNS text LANGUAGE sql AS $$ SELECT r.label $$;

-- Issue #580 variant B / issue #579: function returning SETOF a view's row type.
CREATE TABLE t (id int, name text);
CREATE VIEW tnu_index_v AS SELECT id, name FROM t;
CREATE FUNCTION gettnu(tnu_name text) RETURNS SETOF tnu_index_v LANGUAGE sql AS $$
SELECT * FROM tnu_index_v WHERE name ~ tnu_name $$;

-- Issue #580 follow-up: SQL-language function whose body queries a view, and
-- an aggregate whose input type is a view's row type (with a transition
-- function that takes the row type too). Both are ordered by the diff package.
CREATE FUNCTION count_tnu() RETURNS bigint LANGUAGE sql AS $$ SELECT count(*) FROM tnu_index_v $$;
CREATE FUNCTION tnu_sfunc(state int, r tnu_index_v) RETURNS int LANGUAGE sql IMMUTABLE AS $$ SELECT state + r.id $$;
CREATE AGGREGATE sum_tnu(tnu_index_v) (SFUNC = tnu_sfunc, STYPE = int, INITCOND = '0');
`
	if _, err := conn.ExecContext(ctx, setup); err != nil {
		t.Fatalf("Failed to set up schema: %v", err)
	}

	outDir := t.TempDir()
	mainPath := filepath.Join(outDir, "main.sql")
	if _, err := ExecuteDump(&DumpConfig{
		Host:      host,
		Port:      port,
		DB:        dbname,
		User:      user,
		Password:  password,
		Schema:    "public",
		MultiFile: true,
		File:      mainPath,
	}); err != nil {
		t.Fatalf("Dump command failed: %v", err)
	}

	mainContent, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("Failed to read main.sql: %v", err)
	}
	var includes []string
	for line := range strings.SplitSeq(string(mainContent), "\n") {
		if inc, ok := strings.CutPrefix(line, `\i `); ok {
			includes = append(includes, inc)
		}
	}
	t.Logf("include order:\n%s", strings.Join(includes, "\n"))

	position := func(file string) int {
		for i, inc := range includes {
			if inc == file {
				return i
			}
		}
		t.Fatalf("main.sql does not include %s", file)
		return -1
	}
	mustPrecede := func(before, after string) {
		t.Helper()
		if position(before) > position(after) {
			t.Errorf("expected %s to be included before %s", before, after)
		}
	}

	// Cross-category dependencies from issue #580 / #579.
	mustPrecede("tables/base_table.sql", "functions/use_table_row.sql")
	mustPrecede("views/tnu_index_v.sql", "functions/gettnu.sql")
	// Dependencies that must keep working.
	mustPrecede("functions/touch.sql", "tables/audited.sql")
	mustPrecede("functions/current_owner.sql", "tables/docs.sql")
	mustPrecede("functions/is_ok.sql", "domains/ok_text.sql")
	mustPrecede("domains/ok_text.sql", "tables/uses_domain.sql")
	mustPrecede("functions/log_change.sql", "tables/watched.sql")
	// Body-based dependency of a SQL-language function (#530 ordering).
	mustPrecede("functions/default_label.sql", "tables/labeled.sql")
	mustPrecede("tables/labeled.sql", "functions/count_labeled.sql")
	// Diff-package ordering for view dependencies (#580 follow-up).
	mustPrecede("views/tnu_index_v.sql", "functions/count_tnu.sql")
	mustPrecede("views/tnu_index_v.sql", "functions/tnu_sfunc.sql")
	mustPrecede("functions/tnu_sfunc.sql", "aggregates/sum_tnu.sql")

	// Replay the multi-file dump into an empty schema in include order.
	if _, err := conn.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("Failed to reset schema: %v", err)
	}
	for _, inc := range includes {
		sqlBytes, err := os.ReadFile(filepath.Join(outDir, inc))
		if err != nil {
			t.Fatalf("Failed to read %s: %v", inc, err)
		}
		if _, err := conn.ExecContext(ctx, string(sqlBytes)); err != nil {
			t.Errorf("replaying %s failed: %v", inc, err)
		}
	}
}
