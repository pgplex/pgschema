package plan

import (
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// Concurrent reindexing keeps PostgreSQL in charge of index names, dependency
// transfer and uniqueness enforcement. Never route recovery through DROP/CREATE.
func generateIndexRecovery(index *ir.Index) []RewriteStep {
	qualified := ir.QuoteIdentifier(index.Schema) + "." + ir.QuoteIdentifier(index.Name)
	table := ir.QuoteIdentifier(index.Schema) + "." + ir.QuoteIdentifier(index.Table)
	literal := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	indexLiteral, tableLiteral := literal(qualified), literal(table)
	failure := literal("index " + qualified + " changed or an index build/conflicting operation is active; wait for it to finish and regenerate the plan")
	precondition := fmt.Sprintf(`BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_index i
        JOIN pg_catalog.pg_class c ON c.oid = i.indexrelid
        WHERE i.indexrelid = pg_catalog.to_regclass(%s)
          AND i.indrelid = pg_catalog.to_regclass(%s)
          AND NOT i.indisvalid AND i.indislive AND c.relkind = 'i'
          AND NOT EXISTS (SELECT 1 FROM pg_catalog.pg_constraint con
                          WHERE con.conindid = i.indexrelid AND con.contype IN ('p', 'u', 'x'))
    ) OR EXISTS (
        SELECT 1 FROM pg_catalog.pg_stat_progress_create_index p
        WHERE p.datid = (SELECT oid FROM pg_catalog.pg_database WHERE datname = current_database())
          AND p.relid = pg_catalog.to_regclass(%s)
    ) OR EXISTS (
        SELECT 1 FROM pg_catalog.pg_locks l
        WHERE l.database = (SELECT oid FROM pg_catalog.pg_database WHERE datname = current_database())
          AND l.relation = pg_catalog.to_regclass(%s)
          AND l.mode = 'ShareUpdateExclusiveLock' AND l.granted
          AND l.pid IS DISTINCT FROM pg_backend_pid()
    ) THEN
        RAISE EXCEPTION USING MESSAGE = %s;
    END IF;
END`, indexLiteral, tableLiteral, tableLiteral, tableLiteral, failure)
	postcondition := fmt.Sprintf(`BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_catalog.pg_index i
        WHERE i.indexrelid = pg_catalog.to_regclass(%s)
          AND i.indrelid = pg_catalog.to_regclass(%s)
          AND i.indisvalid AND i.indisready AND i.indislive
    ) THEN
        RAISE EXCEPTION USING MESSAGE = %s;
    END IF;
END`, indexLiteral, tableLiteral, literal("index "+qualified+" is still not valid and ready after recovery; inspect PostgreSQL errors and regenerate the plan"))
	return []RewriteStep{
		{SQL: recoveryAssertion(precondition), CanRunInTransaction: true, RequiresIsolation: true},
		{SQL: "REINDEX INDEX CONCURRENTLY " + qualified + ";", CanRunInTransaction: false},
		{SQL: recoveryAssertion(postcondition), CanRunInTransaction: true, RequiresIsolation: true},
	}
}

// Choose a delimiter absent from the body, including quoted identifiers and
// error messages. A legal index name can itself contain "$pgschema$".
func recoveryAssertion(body string) string {
	tag := "$pgschema$"
	for strings.Contains(body, tag) {
		tag = strings.TrimSuffix(tag, "$") + "_$"
	}
	return "DO " + tag + "\n" + body + "\n" + tag + ";"
}
