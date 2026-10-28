package diff

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/ir/queries"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// Compare native catalog definitions rather than generating the expected SQL
// with the same renderer that produces the migration.
func constraintIncludeCatalog(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT t.relname || ':' || c.conname || ':' || pg_get_constraintdef(c.oid) || ':' || pg_get_indexdef(c.conindid)
 FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid
 WHERE t.relnamespace = 'public'::regnamespace AND c.contype IN ('p', 'u') ORDER BY t.relname, c.conname`)
	require.NoError(t, err)
	defer rows.Close()
	var definitions []string
	for rows.Next() {
		var definition string
		require.NoError(t, rows.Scan(&definition))
		definitions = append(definitions, definition)
	}
	require.NoError(t, rows.Err())
	return definitions
}

func TestConstraintIncludeRoundTrip(t *testing.T) {
	const source = `
 CREATE TABLE ordinary (id int, a int, payload text, "select" text,
 CONSTRAINT ordinary_pk PRIMARY KEY (a, id) INCLUDE ("select", payload) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT ordinary_unique UNIQUE (id, a) INCLUDE (payload, "select") DEFERRABLE);
 CREATE TABLE events (id int, a int, payload text, "select" text,
 CONSTRAINT root_unique UNIQUE (id, a) INCLUDE (payload, "select"),
 CONSTRAINT root_pk PRIMARY KEY (a, id) INCLUDE ("select", payload)) PARTITION BY RANGE (id);
 CREATE TABLE events_low PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (a);
 CREATE TABLE events_leaf PARTITION OF events_low FOR VALUES FROM (0) TO (10);
 DO $$ DECLARE old_name text; BEGIN
 SELECT conname INTO old_name FROM pg_constraint WHERE conrelid = 'events_low'::regclass AND contype = 'u';
 EXECUTE format('ALTER TABLE events_low RENAME CONSTRAINT %I TO "renamed middle unique"', old_name);
 SELECT conname INTO old_name FROM pg_constraint WHERE conrelid = 'events_leaf'::regclass AND contype = 'p';
 EXECUTE format('ALTER TABLE events_leaf RENAME CONSTRAINT %I TO "renamed leaf pk"', old_name);
 END $$;
 `
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer db.Close()
	original := testutil.ParseSQLToIR(t, sharedTestPostgres, source, "public")
	rows, err := queries.New(db).GetConstraints(context.Background())
	require.NoError(t, err)
	found := false
	for _, row := range rows {
		if row.ConstraintName == "ordinary_pk" {
			require.Equal(t, []string{"select", "payload"}, row.IncludeColumns)
			found = true
		}
	}
	require.True(t, found)
	require.Equal(t, constraintColumns("a", "id"), original.Schemas["public"].Tables["ordinary"].Constraints["ordinary_pk"].Columns)
	expected := constraintIncludeCatalog(t, db)
	for _, mode := range []string{"dump", "initial migration"} {
		t.Run(mode, func(t *testing.T) {
			var statements []Diff
			if mode == "dump" {
				var err error
				statements, err = GenerateDump(original, "public", false)
				require.NoError(t, err)
			} else {
				statements = GenerateMigration(&ir.IR{}, original, "public")
			}
			sqlText := buildSQLFromSteps(statements)
			require.Contains(t, sqlText, `UNIQUE (id, a) INCLUDE (payload, "select")`)
			replayed := testutil.ParseSQLToIR(t, sharedTestPostgres, sqlText, "public")
			require.Equal(t, expected, constraintIncludeCatalog(t, db))
			require.Empty(t, GenerateMigration(replayed, original, "public"))
			// Included values must not enlarge the uniqueness key.
			_, err := db.Exec(`INSERT INTO events VALUES (1, 1, 'first', 'x'); INSERT INTO events VALUES (1, 1, 'second', 'y')`)
			require.Error(t, err)
		})
	}
}

func TestConstraintIncludeMigration(t *testing.T) {
	for _, topology := range []string{"ordinary", "partitioned"} {
		for _, kind := range []string{"PRIMARY KEY", "UNIQUE"} {
			for _, transition := range []struct{ name, old, new string }{
				{"add", "", ` INCLUDE (payload, "select")`},
				{"reorder", ` INCLUDE (payload, "select")`, ` INCLUDE ("select", payload)`},
				{"change", ` INCLUDE (payload)`, ` INCLUDE ("select")`},
				{"remove", ` INCLUDE (payload)`, ""},
			} {
				t.Run(topology+"/"+kind+"/"+transition.name, func(t *testing.T) {
					schema := func(include string) string {
						partition := ""
						if topology == "partitioned" {
							partition = ` PARTITION BY RANGE (id); CREATE TABLE leaf PARTITION OF parent FOR VALUES FROM (0) TO (10)`
						}
						return `CREATE TABLE parent (id int NOT NULL, payload text, "select" text, CONSTRAINT parent_key ` + kind + ` (id)` + include + `)` + partition + `; CREATE TABLE child (id int REFERENCES parent(id));`
					}
					desired := testutil.ParseSQLToIR(t, sharedTestPostgres, schema(transition.new), "public")
					db, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
					defer db.Close()
					expected := constraintIncludeCatalog(t, db)
					old := testutil.ParseSQLToIR(t, sharedTestPostgres, schema(transition.old), "public")
					steps := GenerateMigration(old, desired, "public")
					require.NotEmpty(t, steps)
					_, err := db.Exec(buildSQLFromSteps(steps))
					require.NoError(t, err)
					require.Equal(t, expected, constraintIncludeCatalog(t, db))
					actual, err := ir.NewInspector(db, nil).BuildIR(context.Background(), "public")
					require.NoError(t, err)
					require.Empty(t, GenerateMigration(actual, desired, "public"))
				})
			}
		}
	}
}

func TestConstraintIncludeColumnChanges(t *testing.T) {
	for _, kind := range []string{"PRIMARY KEY", "UNIQUE"} {
		for _, change := range []struct{ name, old, new string }{
			{"add constraint", `CREATE TABLE parent (id int NOT NULL, payload text);`, `CREATE TABLE parent (id int NOT NULL, payload text, CONSTRAINT parent_key ` + kind + ` (id) INCLUDE (payload));`},
			{"add columns", `CREATE TABLE parent (existing int);`, `CREATE TABLE parent (existing int, id int, payload text, CONSTRAINT parent_key ` + kind + ` (id) INCLUDE (payload));`},
			{"drop included column", `CREATE TABLE parent (id int NOT NULL, payload text, CONSTRAINT parent_key ` + kind + ` (id) INCLUDE (payload));`, `CREATE TABLE parent (id int NOT NULL, CONSTRAINT parent_key ` + kind + ` (id));`},
		} {
			t.Run(kind+"/"+change.name, func(t *testing.T) {
				desired := testutil.ParseSQLToIR(t, sharedTestPostgres, change.new, "public")
				db, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
				defer db.Close()
				expected := constraintIncludeCatalog(t, db)
				old := testutil.ParseSQLToIR(t, sharedTestPostgres, change.old, "public")
				_, err := db.Exec(buildSQLFromSteps(GenerateMigration(old, desired, "public")))
				require.NoError(t, err)
				require.Equal(t, expected, constraintIncludeCatalog(t, db))
				actual, err := ir.NewInspector(db, nil).BuildIR(context.Background(), "public")
				require.NoError(t, err)
				require.Empty(t, GenerateMigration(actual, desired, "public"))
			})
		}
	}
}

func TestConstraintIncludeAttachDetach(t *testing.T) {
	const attached = `CREATE TABLE parent (id int, payload text, "select" text,
 CONSTRAINT parent_pk PRIMARY KEY (id) INCLUDE ("select", payload),
 CONSTRAINT parent_unique UNIQUE (id) INCLUDE (payload, "select")) PARTITION BY RANGE (id);
 CREATE TABLE leaf PARTITION OF parent FOR VALUES FROM (0) TO (10);`
	const detached = attached + ` ALTER TABLE parent DETACH PARTITION leaf;`
	for _, change := range []struct{ name, old, new string }{
		{"detach", attached, detached}, {"attach", detached, attached},
		{"attach different INCLUDE", detached + ` ALTER TABLE leaf DROP CONSTRAINT leaf_pkey; ALTER TABLE leaf ADD CONSTRAINT leaf_pkey PRIMARY KEY (id) INCLUDE (payload);`, attached},
	} {
		t.Run(change.name, func(t *testing.T) {
			desired := testutil.ParseSQLToIR(t, sharedTestPostgres, change.new, "public")
			db, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
			defer db.Close()
			expected := constraintIncludeCatalog(t, db)
			old := testutil.ParseSQLToIR(t, sharedTestPostgres, change.old, "public")
			_, err := db.Exec(buildSQLFromSteps(GenerateMigration(old, desired, "public")))
			require.NoError(t, err)
			require.Equal(t, expected, constraintIncludeCatalog(t, db))
			actual, err := ir.NewInspector(db, nil).BuildIR(context.Background(), "public")
			require.NoError(t, err)
			require.Empty(t, GenerateMigration(actual, desired, "public"))
		})
	}
}

func TestConstraintIncludeComparison(t *testing.T) {
	for _, kind := range []ir.ConstraintType{ir.ConstraintTypePrimaryKey, ir.ConstraintTypeUnique} {
		original := &ir.Constraint{Name: "key", Type: kind, Columns: constraintColumns("id"), IncludeColumns: []string{"payload", "select"}}
		for _, included := range [][]string{nil, {"payload"}, {"select", "payload"}, {"payload", "select"}} {
			changed := *original
			changed.IncludeColumns = included
			equal := len(included) == 2 && included[0] == "payload"
			require.Equal(t, equal, constraintsEqual(original, &changed))
			require.Equal(t, equal, parentSuppliesConstraint([]*ir.Constraint{original}, &changed))
		}
	}
}
