package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// CREATE TABLE silently drops a key whose index equals an earlier one, so the
// generated SQL must add each such key after the CREATE TABLE (issue #446).
func TestDuplicateIndexConstraintRoundTrip(t *testing.T) {
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	defer db.Close()
	majorVersion, err := testutil.GetMajorVersion(db)
	require.NoError(t, err)

	source := `
 CREATE TABLE products (sku text PRIMARY KEY, name text, a int, b int,
 CONSTRAINT products_name_key UNIQUE (name),
 CONSTRAINT products_a_b_key UNIQUE (a, b),
 CONSTRAINT products_b_a_key UNIQUE (b, a),
 CONSTRAINT products_sku_include UNIQUE (sku) INCLUDE (name),
 CONSTRAINT products_sku_deferrable UNIQUE (sku) DEFERRABLE,
 CONSTRAINT products_sku_deferred UNIQUE (sku) DEFERRABLE INITIALLY DEFERRED,
 CONSTRAINT products_a_excl EXCLUDE USING btree (a WITH =),
 CONSTRAINT products_b_excl EXCLUDE USING btree (b WITH =) WITH (fillfactor=70));
 ALTER TABLE products ADD CONSTRAINT products_sku_key UNIQUE (sku);
 ALTER TABLE products ADD CONSTRAINT products_name_unique UNIQUE (name);
 ALTER TABLE products ADD CONSTRAINT products_sku_deferrable2 UNIQUE (sku) DEFERRABLE;
 ALTER TABLE products ADD CONSTRAINT products_a_excl2 EXCLUDE USING btree (a WITH =);
 ALTER TABLE products ADD CONSTRAINT products_b_excl2 EXCLUDE USING btree (b WITH =);
 COMMENT ON CONSTRAINT products_sku_key ON products IS 'target of order_lines_sku_fkey';
 CREATE TABLE order_lines (id int PRIMARY KEY, sku text REFERENCES products (sku));
 CREATE TABLE events (id int) PARTITION BY RANGE (id);
 CREATE TABLE events_leaf PARTITION OF events (CONSTRAINT events_leaf_pkey PRIMARY KEY (id)) FOR VALUES FROM (0) TO (10);
 ALTER TABLE events_leaf ADD CONSTRAINT events_leaf_id_key UNIQUE (id);
 `
	if majorVersion >= 15 {
		source += `ALTER TABLE products ADD CONSTRAINT products_name_nulls_key UNIQUE NULLS NOT DISTINCT (name);`
	}
	original := testutil.ParseSQLToIR(t, sharedTestPostgres, source, "public")

	for _, mode := range []string{"dump", "qualified dump", "initial migration"} {
		t.Run(mode, func(t *testing.T) {
			var statements []Diff
			var err error
			prefix := ""
			switch mode {
			case "dump":
				statements, err = GenerateDump(original, "public", false)
				require.NoError(t, err)
			case "qualified dump":
				statements, err = GenerateDump(original, "public", true)
				require.NoError(t, err)
				prefix = "public."
			default:
				statements = GenerateMigration(&ir.IR{}, original, "public")
			}
			added := []string{
				"ALTER TABLE " + prefix + "products\nADD CONSTRAINT products_a_excl2 EXCLUDE USING btree (a WITH =);",
				// PostgreSQL ignores the storage parameters when it compares the indexes.
				"ALTER TABLE " + prefix + "products\nADD CONSTRAINT products_b_excl2 EXCLUDE USING btree (b WITH =);",
				"ALTER TABLE " + prefix + "products\nADD CONSTRAINT products_name_unique UNIQUE (name);",
				"ALTER TABLE " + prefix + "products\nADD CONSTRAINT products_sku_deferrable2 UNIQUE (sku) DEFERRABLE;",
				"ALTER TABLE " + prefix + "products\nADD CONSTRAINT products_sku_key UNIQUE (sku);",
				"ALTER TABLE " + prefix + "events_leaf\nADD CONSTRAINT events_leaf_id_key UNIQUE (id);",
			}
			sqlText := buildSQLFromSteps(statements)
			for _, statement := range added {
				require.Contains(t, sqlText, statement)
			}
			require.Equal(t, len(added), strings.Count(sqlText, "ADD CONSTRAINT"), sqlText)
			comment := strings.Index(sqlText, "COMMENT ON CONSTRAINT products_sku_key")
			require.Greater(t, comment, strings.Index(sqlText, added[4]))

			replayed := testutil.ParseSQLToIR(t, sharedTestPostgres, sqlText, "public")
			require.Empty(t, GenerateMigration(replayed, original, "public"))
		})
	}
}
