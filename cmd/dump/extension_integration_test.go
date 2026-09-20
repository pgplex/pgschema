package dump

import (
	"context"
	"fmt"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// Issue #595: pg_stat_statements' view was dumped while its required function
// was omitted. Neither definition belongs in an application schema dump.
func TestDumpExtensionMembers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, host, port, name, user, password := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	ctx := context.Background()
	// Catalog inspection and view creation do not execute pg_stat_statements,
	// so the bundled extension needs no shared_preload_libraries modification.
	_, err := db.ExecContext(ctx, `
		CREATE EXTENSION pg_stat_statements;
		CREATE TABLE app_requests (id integer PRIMARY KEY);
		CREATE VIEW app_stats AS SELECT queryid FROM pg_stat_statements;
		GRANT SELECT ON app_requests TO PUBLIC;
		GRANT SELECT (queryid) ON pg_stat_statements TO PUBLIC;
	`)
	require.NoError(t, err)
	config := &DumpConfig{
		Host: host, Port: port, DB: name, User: user, Password: password,
		Schema: "public", NoComments: true,
	}
	dumped, err := ExecuteDump(config)
	require.NoError(t, err)
	require.NotContains(t, dumped, "VIEW pg_stat_statements")
	require.NotContains(t, dumped, "CREATE OR REPLACE FUNCTION pg_stat_statements")
	require.NotContains(t, dumped, "ON TABLE pg_stat_statements")
	require.Contains(t, dumped, "CREATE OR REPLACE VIEW app_stats")
	require.Contains(t, dumped, "FROM pg_stat_statements")
	require.Contains(t, dumped, "GRANT SELECT ON TABLE app_requests TO PUBLIC")
	// Reload the unmodified native dump while the extension is still installed.
	_, err = db.ExecContext(ctx, `DROP VIEW app_stats; DROP TABLE app_requests;`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, dumped)
	require.NoError(t, err)
	roundtrip, err := ExecuteDump(config)
	require.NoError(t, err)
	require.Equal(t, dumped, roundtrip)
}

// An application partition remains managed even when its parent is an
// extension member. Replaying its dump must preserve its own column rules.
func TestDumpExtensionPartitionOverrides(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	pg := testutil.SetupPostgres(t)
	defer pg.Stop()
	db, host, port, name, user, password := testutil.ConnectToPostgres(t, pg)
	defer db.Close()
	ctx := context.Background()
	_, err := db.ExecContext(ctx, `CREATE SCHEMA extension595; CREATE EXTENSION hstore SCHEMA extension595;`)
	require.NoError(t, err)
	for _, tc := range []struct{ parentSchema, childSchema string }{
		{"public", "public"},
		{"Extension Space", "App Space"},
	} {
		t.Run(tc.childSchema, func(t *testing.T) {
			parent := ir.QuoteIdentifier(tc.parentSchema) + `."Member Parent"`
			child := ir.QuoteIdentifier(tc.childSchema) + `."App Child"`
			_, err := db.ExecContext(ctx, fmt.Sprintf(`
				CREATE SCHEMA IF NOT EXISTS %s;
				CREATE SCHEMA IF NOT EXISTS %s;
				CREATE TABLE %s (
					id integer NOT NULL,
					priority integer DEFAULT 0,
					notes text,
					inherited integer DEFAULT 42 NOT NULL,
					calculated integer GENERATED ALWAYS AS (id * 2) STORED
				) PARTITION BY RANGE (id);
				ALTER EXTENSION hstore ADD TABLE %s;
				CREATE TABLE %s PARTITION OF %s (
					priority DEFAULT 10, notes NOT NULL
				) FOR VALUES FROM (0) TO (100);
			`, ir.QuoteIdentifier(tc.parentSchema), ir.QuoteIdentifier(tc.childSchema), parent, parent, child, parent))
			require.NoError(t, err)
			config := &DumpConfig{Host: host, Port: port, DB: name, User: user, Password: password,
				Schema: tc.childSchema, NoComments: true, QualifySchema: true}
			dumped, err := ExecuteDump(config)
			require.NoError(t, err)
			require.NotContains(t, dumped, `CREATE TABLE IF NOT EXISTS `+parent+` (`)
			_, err = db.ExecContext(ctx, `DROP TABLE `+child)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, dumped)
			require.NoError(t, err, dumped)
			var priority, inherited, calculated int
			err = db.QueryRowContext(ctx, `INSERT INTO `+child+` (id, notes) VALUES (1, 'kept') RETURNING priority, inherited, calculated`).Scan(&priority, &inherited, &calculated)
			require.NoError(t, err)
			require.Equal(t, 10, priority, "partition default must survive dump replay")
			require.Equal(t, 42, inherited)
			require.Equal(t, 2, calculated)
			_, err = db.ExecContext(ctx, `INSERT INTO `+child+` (id) VALUES (2)`)
			require.ErrorContains(t, err, "23502", "partition NOT NULL must survive dump replay")
			roundtrip, err := ExecuteDump(config)
			require.NoError(t, err)
			require.Equal(t, dumped, roundtrip)
			_, err = db.ExecContext(ctx, `DROP TABLE `+child+`; ALTER EXTENSION hstore DROP TABLE `+parent+`; DROP TABLE `+parent)
			require.NoError(t, err)
		})
	}
}
