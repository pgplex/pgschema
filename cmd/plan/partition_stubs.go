package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// prependPartitionParentStubs creates stub CREATE TABLE ... PARTITION BY
// statements for cross-schema partition parents referenced by PARTITION OF
// in the desired SQL. The stubs use unqualified names so they land in the
// temp schema via search_path, and the PARTITION OF references in the
// desired SQL are rewritten to also be unqualified. This avoids creating
// or mutating persistent objects outside the temp schema in the plan database.
func prependPartitionParentStubs(ctx context.Context, cfg *util.ConnectionConfig, targetSchema, desiredSQL string) (string, error) {
	refs := postgres.ExtractPartitionOfTargets(desiredSQL, targetSchema)
	if len(refs) == 0 {
		return desiredSQL, nil
	}

	created := make(map[string]bool)
	for _, name := range postgres.ExtractCreateTableNames(desiredSQL, targetSchema) {
		created[name.Schema+"."+name.Table] = true
	}

	var toStub []postgres.QualifiedName
	seen := make(map[string]bool)
	for _, ref := range refs {
		if ref.Schema == targetSchema {
			continue
		}
		key := ref.Schema + "." + ref.Table
		if created[key] || seen[key] {
			continue
		}
		seen[key] = true
		toStub = append(toStub, ref)
	}
	if len(toStub) == 0 {
		return desiredSQL, nil
	}

	conn, err := util.Connect(cfg)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	var stubs strings.Builder
	rewritten := desiredSQL
	for _, ref := range toStub {
		ddl, err := ir.BuildPartitionedTableStubSQL(ctx, conn, ref.Schema, ref.Table)
		if err != nil {
			return "", err
		}
		if ddl == "" {
			logger.Get().Warn("cross-schema partition parent not found or not partitioned",
				"schema", ref.Schema, "table", ref.Table)
			return "", fmt.Errorf("cross-schema partition parent %s.%s not found or not a partitioned table on the target database", ref.Schema, ref.Table)
		}
		logger.Get().Debug("prepending stub for cross-schema partition parent",
			"schema", ref.Schema, "table", ref.Table)
		stubs.WriteString(ddl)

		// Rewrite PARTITION OF references to strip the cross-schema prefix so
		// they resolve to the stub in the temp schema via search_path.
		rewritten = stripPartitionOfSchema(rewritten, ref.Schema, ref.Table)
	}

	return stubs.String() + rewritten, nil
}

// stripPartitionOfSchema rewrites "PARTITION OF schema.table" to
// "PARTITION OF table" in SQL, handling both quoted and unquoted identifiers.
func stripPartitionOfSchema(sql, schema, table string) string {
	// Build the qualified reference as it appears in SQL.
	// Handle both quoted and unquoted forms.
	patterns := []struct{ old, new string }{
		// unquoted: schema.table
		{
			old: strings.ToLower(schema) + "." + strings.ToLower(table),
			new: strings.ToLower(table),
		},
	}

	// Also handle quoted schema: "schema".table, "schema"."table", schema."table"
	quotedSchema := ir.QuoteIdentifier(schema)
	quotedTable := ir.QuoteIdentifier(table)
	if quotedSchema != schema {
		patterns = append(patterns,
			struct{ old, new string }{quotedSchema + "." + table, table},
			struct{ old, new string }{quotedSchema + "." + quotedTable, quotedTable},
		)
	}
	if quotedTable != table {
		patterns = append(patterns,
			struct{ old, new string }{schema + "." + quotedTable, quotedTable},
		)
	}

	result := sql
	for _, p := range patterns {
		result = replacePartitionOfRef(result, p.old, p.new)
	}
	return result
}

// replacePartitionOfRef replaces "PARTITION OF <oldRef>" with
// "PARTITION OF <newRef>" in SQL, case-insensitively matching the
// PARTITION OF keywords.
func replacePartitionOfRef(sql, oldRef, newRef string) string {
	lower := strings.ToLower(sql)
	lowerOld := strings.ToLower(oldRef)
	var b strings.Builder
	i := 0
	for i < len(sql) {
		// Find "partition" keyword
		idx := strings.Index(lower[i:], "partition")
		if idx < 0 {
			b.WriteString(sql[i:])
			break
		}
		pos := i + idx
		b.WriteString(sql[i:pos])

		// Check if it's followed by whitespace + "of" + whitespace + oldRef
		rest := pos + len("partition")
		j := rest
		for j < len(sql) && (sql[j] == ' ' || sql[j] == '\t' || sql[j] == '\n' || sql[j] == '\r') {
			j++
		}
		if j+2 <= len(sql) && strings.EqualFold(sql[j:j+2], "of") {
			afterOf := j + 2
			k := afterOf
			for k < len(sql) && (sql[k] == ' ' || sql[k] == '\t' || sql[k] == '\n' || sql[k] == '\r') {
				k++
			}
			if k+len(lowerOld) <= len(sql) && strings.EqualFold(sql[k:k+len(lowerOld)], lowerOld) {
				// Matched — write "PARTITION OF <newRef>" preserving original case of keywords
				b.WriteString(sql[pos:afterOf])
				b.WriteString(sql[afterOf:k])
				b.WriteString(newRef)
				i = k + len(lowerOld)
				continue
			}
		}
		b.WriteString(sql[pos : pos+len("partition")])
		i = pos + len("partition")
	}
	return b.String()
}
