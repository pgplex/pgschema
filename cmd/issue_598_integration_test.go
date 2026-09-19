package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pgplex/pgschema/cmd/apply"
	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const issue598Tables = `
CREATE TABLE parents (id integer PRIMARY KEY);
CREATE TABLE items (
    id integer PRIMARY KEY,
    code integer,
    payload text NOT NULL UNIQUE,
    parent_id integer REFERENCES parents(id)
);`

// TestIssue598InterruptedConcurrentIndex exercises an actual interrupted native
// plan/apply, rather than manufacturing an invalid catalog entry or repairing it
// with SQL outside pgschema. The writer transaction makes interruption repeatable
// even on small tables and fast machines.
func TestIssue598InterruptedConcurrentIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()

	for _, unique := range []bool{false, true} {
		name, modifier := "ordinary", ""
		if unique {
			name, modifier = "unique", "UNIQUE "
		}
		t.Run(name, func(t *testing.T) {
			indexSQL := fmt.Sprintf("CREATE %sINDEX items_code_idx ON items (code) INCLUDE (payload) WITH (fillfactor=80) WHERE code > 0;", modifier)
			if unique {
				indexSQL += "\nCOMMENT ON INDEX items_code_idx IS 'issue 598 preserved comment';"
			}
			f := newIssue598Fixture(t, provider, indexSQL)
			initial := f.generate(t)
			require.Contains(t, initial.ToSQL(plan.SQLFormatRaw), "INDEX CONCURRENTLY", "fixture must exercise native online creation")
			beforeRows := f.rows(t)
			writer, err := f.db.BeginTx(context.Background(), nil)
			require.NoError(t, err)
			defer writer.Rollback()
			_, err = writer.Exec("UPDATE items SET payload = payload WHERE id = 1")
			require.NoError(t, err)

			result := make(chan error, 1)
			config := f.savedApplyConfig(t, initial)
			go func() { result <- apply.ApplyMigration(config, nil) }()
			pid := f.waitForBuild(t, result)

			// While the invalid index belongs to a live build, a second plan must
			// refuse to treat it as an abandoned index ready for recovery.
			_, activeErr := planCmd.GeneratePlan(f.config, provider)
			if assert.Error(t, activeErr, "planning must refuse an active concurrent index build") {
				message := strings.ToLower(activeErr.Error())
				assert.True(t, strings.Contains(message, "active") || strings.Contains(message, "conflict") || strings.Contains(message, "wait"), "actionable active-build error: %v", activeErr)
			}

			var cancelled bool
			require.NoError(t, f.db.QueryRow("SELECT pg_cancel_backend($1)", pid).Scan(&cancelled))
			require.True(t, cancelled)
			select {
			case err := <-result:
				require.Error(t, err, "interrupted concurrent create must report failure")
			case <-time.After(10 * time.Second):
				t.Fatal("native apply did not finish after cancelling its exact backend")
			}
			require.NoError(t, writer.Rollback())
			definition := f.assertIndex(t, false)
			require.Equal(t, beforeRows, f.rows(t), "interruption must preserve every row")

			recovery := f.generate(t)
			require.NotEmpty(t, recovery.Groups, "an abandoned invalid index must not be reported as converged")
			require.Contains(t, recovery.ToSQL(plan.SQLFormatRaw), "REINDEX INDEX CONCURRENTLY", "recovery must retain the existing index until its replacement is ready")
			require.NoError(t, f.apply(t, recovery))
			require.Equal(t, definition, f.assertIndex(t, true), "recovery must preserve the complete PostgreSQL index definition")
			require.Equal(t, beforeRows, f.rows(t))
			if unique {
				var comment string
				require.NoError(t, f.db.QueryRow("SELECT obj_description('items_code_idx'::regclass, 'pg_class')").Scan(&comment))
				require.Equal(t, "issue 598 preserved comment", comment)
			}
			f.assertConstraints(t)
			require.Empty(t, f.generate(t).Groups, "repeat native plan must converge only after the index is healthy")
		})
	}
}

// Failed uniqueness checks must remain visible and leave data correction to the
// operator. Only the explicit fixture UPDATE below changes application data.
func TestIssue598DuplicateDataRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	f := newIssue598Fixture(t, provider, "CREATE UNIQUE INDEX items_code_idx ON items (code);")
	_, err := f.db.Exec("UPDATE items SET code = 1 WHERE id = 2")
	require.NoError(t, err)
	beforeRows := f.rows(t)
	require.Error(t, f.apply(t, f.generate(t)), "duplicate data must fail native concurrent creation")
	definition := f.assertIndex(t, false)

	// PostgreSQL must not allow an unusable unique index to become a constraint.
	_, err = f.db.Exec("ALTER TABLE items ADD CONSTRAINT broken_unique UNIQUE USING INDEX items_code_idx")
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "valid")

	recovery := f.generate(t)
	require.NotEmpty(t, recovery.Groups, "failed unique creation must produce a recovery plan")
	require.Contains(t, recovery.ToSQL(plan.SQLFormatRaw), "REINDEX INDEX CONCURRENTLY")
	require.Error(t, f.apply(t, recovery), "recovery must fail while duplicates remain")
	require.Equal(t, beforeRows, f.rows(t), "failed recovery must not remove or alter rows")
	require.Equal(t, definition, f.assertIndex(t, false))
	f.assertConstraints(t)

	_, err = f.db.Exec("UPDATE items SET code = 2 WHERE id = 2")
	require.NoError(t, err, "explicit fixture data correction")
	correctedRows := f.rows(t)
	require.NoError(t, f.apply(t, f.generate(t)), "a fresh plan must recover after explicit data correction")
	require.Equal(t, definition, f.assertIndex(t, true))
	require.Equal(t, correctedRows, f.rows(t))
	f.assertConstraints(t)
	require.Empty(t, f.generate(t).Groups)
}

type issue598Fixture struct {
	db       *sql.DB
	provider *postgres.EmbeddedPostgres
	config   *planCmd.PlanConfig
}

func newIssue598Fixture(t *testing.T, provider *postgres.EmbeddedPostgres, indexSQL string) *issue598Fixture {
	t.Helper()
	target := testutil.SetupPostgres(t)
	t.Cleanup(func() { target.Stop() })
	db, host, port, dbname, user, password := testutil.ConnectToPostgres(t, target)
	t.Cleanup(func() { db.Close() })
	_, err := db.Exec(issue598Tables + `
INSERT INTO parents VALUES (1);
INSERT INTO items VALUES (1, 1, 'first', 1), (2, 2, 'second', 1), (3, NULL, 'third', NULL);`)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "desired.sql")
	require.NoError(t, os.WriteFile(file, []byte(issue598Tables+"\n"+indexSQL), 0644))
	return &issue598Fixture{db: db, provider: provider, config: &planCmd.PlanConfig{
		Host: host, Port: port, DB: dbname, User: user, Password: password,
		Schema: "public", File: file, SSLMode: "disable", ApplicationName: "pgschema-issue598",
	}}
}

func (f *issue598Fixture) generate(t *testing.T) *plan.Plan {
	t.Helper()
	p, err := planCmd.GeneratePlan(f.config, f.provider)
	require.NoError(t, err)
	return p
}

func (f *issue598Fixture) savedApplyConfig(t *testing.T, p *plan.Plan) *apply.ApplyConfig {
	t.Helper()
	data, err := json.Marshal(p)
	require.NoError(t, err)
	file := filepath.Join(t.TempDir(), "plan.json")
	require.NoError(t, os.WriteFile(file, data, 0644))
	data, err = os.ReadFile(file)
	require.NoError(t, err)
	loaded, err := plan.FromJSON(data) // The same loader used by apply --plan.
	require.NoError(t, err)
	return &apply.ApplyConfig{
		Host: f.config.Host, Port: f.config.Port, DB: f.config.DB,
		User: f.config.User, Password: f.config.Password, Schema: f.config.Schema,
		Plan: loaded, SSLMode: "disable", ApplicationName: "pgschema-issue598-apply",
		AutoApprove: true, Quiet: true, LockTimeout: "20s",
	}
}

func (f *issue598Fixture) apply(t *testing.T, p *plan.Plan) error {
	t.Helper()
	return apply.ApplyMigration(f.savedApplyConfig(t, p), nil)
}

func (f *issue598Fixture) waitForBuild(t *testing.T, result <-chan error) int {
	t.Helper()
	deadline := time.After(10 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var pid int
		err := f.db.QueryRow(`
SELECT p.pid
FROM pg_stat_progress_create_index p
JOIN pg_stat_activity a ON a.pid = p.pid
WHERE p.datid = (SELECT oid FROM pg_database WHERE datname = current_database())
  AND p.relid = 'public.items'::regclass
  AND p.index_relid = to_regclass('public.items_code_idx')
  AND p.command = 'CREATE INDEX CONCURRENTLY'
  AND p.phase = 'waiting for writers before build'
  AND a.application_name = 'pgschema-issue598-apply'`).Scan(&pid)
		if err == nil {
			return pid
		}
		require.ErrorIs(t, err, sql.ErrNoRows)
		select {
		case err := <-result:
			t.Fatalf("apply ended before the concurrent build reached its writer wait: %v", err)
		case <-deadline:
			t.Fatal("concurrent build never reached the expected writer wait")
		case <-tick.C:
		}
	}
}

func (f *issue598Fixture) assertIndex(t *testing.T, wantValid bool) string {
	t.Helper()
	var valid, ready, live bool
	var definition string
	require.NoError(t, f.db.QueryRow(`SELECT indisvalid, indisready, indislive, pg_get_indexdef(indexrelid)
FROM pg_index WHERE indexrelid = 'public.items_code_idx'::regclass`).Scan(&valid, &ready, &live, &definition))
	require.Equal(t, wantValid, valid, "index validity")
	require.True(t, live, "index must remain live")
	if wantValid {
		require.True(t, ready, "a recovered index must be ready for writes")
	}
	return definition
}

func (f *issue598Fixture) rows(t *testing.T) string {
	t.Helper()
	var rows string
	require.NoError(t, f.db.QueryRow("SELECT json_agg(items ORDER BY id)::text FROM items").Scan(&rows))
	return rows
}

func (f *issue598Fixture) assertConstraints(t *testing.T) {
	t.Helper()
	var constraints, unhealthy int
	require.NoError(t, f.db.QueryRow(`SELECT count(*), count(*) FILTER (WHERE NOT c.convalidated)
FROM pg_constraint c WHERE c.conrelid IN ('public.items'::regclass, 'public.parents'::regclass)
AND c.contype IN ('p', 'u', 'f')`).Scan(&constraints, &unhealthy))
	require.Equal(t, 4, constraints, "both primary keys, unique constraint and foreign key must survive")
	require.Zero(t, unhealthy)
	require.NoError(t, f.db.QueryRow(`SELECT count(*) FROM pg_index i JOIN pg_constraint c ON c.conindid = i.indexrelid
WHERE c.conrelid IN ('public.items'::regclass, 'public.parents'::regclass)
AND (NOT i.indisvalid OR NOT i.indisready OR NOT i.indislive)`).Scan(&unhealthy))
	require.Zero(t, unhealthy, "constraint backing indexes must stay usable")
}

func TestIssue598PartitionStates(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	f := newIssue598Fixture(t, provider, "")
	const partitionSQL = `
CREATE TABLE partition_parent (id integer) PARTITION BY RANGE (id);
CREATE TABLE partition_child PARTITION OF partition_parent FOR VALUES FROM (0) TO (100);
CREATE INDEX parent_index ON ONLY partition_parent(id);`
	_, err := f.db.Exec(partitionSQL)
	require.NoError(t, err)
	desired := issue598Tables + partitionSQL
	require.NoError(t, os.WriteFile(f.config.File, []byte(desired), 0644))
	require.Empty(t, f.generate(t).Groups, "intentional invalid ON ONLY parent is not an abandoned concurrent build")
	maintenance, err := f.db.Begin()
	require.NoError(t, err)
	defer maintenance.Rollback()
	_, err = maintenance.Exec("LOCK TABLE ONLY partition_parent IN SHARE UPDATE EXCLUSIVE MODE")
	require.NoError(t, err)
	require.NoError(t, f.apply(t, f.generate(t)), "maintenance on an intentional invalid parent must allow a no-op")
	desired += "\nALTER TABLE parents ADD COLUMN note text;"
	require.NoError(t, os.WriteFile(f.config.File, []byte(desired), 0644))
	require.NoError(t, f.apply(t, f.generate(t)), "maintenance on an untouched invalid parent must not block unrelated DDL")
	require.NoError(t, maintenance.Rollback())
	var valid, ready, live bool
	require.NoError(t, f.db.QueryRow("SELECT indisvalid,indisready,indislive FROM pg_index WHERE indexrelid='parent_index'::regclass").Scan(&valid, &ready, &live))
	require.False(t, valid)
	require.True(t, ready)
	require.True(t, live)
	require.NoError(t, os.WriteFile(f.config.File, []byte(strings.ReplaceAll(desired, "ON ONLY partition_parent", "ON partition_parent")), 0644))
	_, err = planCmd.GeneratePlan(f.config, provider)
	require.ErrorContains(t, err, "attach", "an incomplete parent must not silently satisfy a desired valid parent")
	require.NoError(t, f.db.QueryRow("SELECT indisvalid FROM pg_index WHERE indexrelid='parent_index'::regclass").Scan(&valid))
	require.False(t, valid)

	// Removing an intentionally incomplete parent needs no attachment repair.
	withoutIndex := strings.ReplaceAll(desired, "CREATE INDEX parent_index ON ONLY partition_parent(id);", "")
	require.NoError(t, os.WriteFile(f.config.File, []byte(withoutIndex), 0644))
	require.NoError(t, f.apply(t, f.generate(t)))
	require.Empty(t, f.generate(t).Groups)
	_, err = f.db.Exec("CREATE INDEX parent_index ON ONLY partition_parent(id)")
	require.NoError(t, err)
	// Recreate the same catalog state and remove the whole partitioned table.
	require.NoError(t, os.WriteFile(f.config.File, []byte(issue598Tables+"\nALTER TABLE parents ADD COLUMN note text;"), 0644))
	require.NoError(t, f.apply(t, f.generate(t)))
	var absent bool
	require.NoError(t, f.db.QueryRow("SELECT to_regclass('partition_parent') IS NULL AND to_regclass('parent_index') IS NULL").Scan(&absent))
	require.True(t, absent)
	require.Empty(t, f.generate(t).Groups)
}

func TestIssue598QuotedIndexRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	const name = `odd'$pgschema$index`
	f := newIssue598Fixture(t, provider, `CREATE UNIQUE INDEX "odd'$pgschema$index" ON items(code);`)
	_, err := f.db.Exec("UPDATE items SET code=1 WHERE id=2")
	require.NoError(t, err)
	require.Error(t, f.apply(t, f.generate(t)), "produce a real invalid index with legal special characters in its name")
	_, err = f.db.Exec("UPDATE items SET code=2 WHERE id=2")
	require.NoError(t, err)
	require.NoError(t, f.apply(t, f.generate(t)), "identifier text must not terminate generated assertion bodies")
	var usable bool
	require.NoError(t, f.db.QueryRow(`SELECT indisvalid AND indisready AND indislive FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid WHERE c.relname=$1`, name).Scan(&usable))
	require.True(t, usable)
	require.Empty(t, f.generate(t).Groups)
}

func TestIssue598PartitionConstraintRemoval(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	provider := testutil.SetupPostgres(t)
	defer provider.Stop()
	f := newIssue598Fixture(t, provider, "")
	const tables = `CREATE TABLE partition_parent (id integer) PARTITION BY RANGE (id);
CREATE TABLE partition_child PARTITION OF partition_parent FOR VALUES FROM (0) TO (100);`
	_, err := f.db.Exec(tables + "ALTER TABLE ONLY partition_parent ADD CONSTRAINT parent_unique UNIQUE(id);")
	require.NoError(t, err)
	var valid bool
	require.NoError(t, f.db.QueryRow("SELECT indisvalid FROM pg_index WHERE indexrelid='parent_unique'::regclass").Scan(&valid))
	require.False(t, valid)
	require.NoError(t, os.WriteFile(f.config.File, []byte(issue598Tables+tables), 0644))
	removal := f.generate(t)
	maintenance, err := f.db.Begin()
	require.NoError(t, err)
	defer maintenance.Rollback()
	_, err = maintenance.Exec("LOCK TABLE ONLY partition_parent IN SHARE UPDATE EXCLUSIVE MODE")
	require.NoError(t, err)
	_, err = planCmd.GeneratePlan(f.config, provider)
	require.ErrorContains(t, err, "maintenance")
	require.ErrorContains(t, f.apply(t, removal), "maintenance", "saved constraint removal must recheck activity")
	require.NoError(t, maintenance.Rollback())
	require.NoError(t, f.apply(t, f.generate(t)))
	require.Empty(t, f.generate(t).Groups)
	_, err = f.db.Exec("ALTER TABLE ONLY partition_parent ADD CONSTRAINT parent_unique UNIQUE(id)")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(f.config.File, []byte(issue598Tables), 0644))
	require.NoError(t, f.apply(t, f.generate(t)))
	require.Empty(t, f.generate(t).Groups)
	f.assertConstraints(t)
}
