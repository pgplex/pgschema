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

// partitionStubPrefix is prepended to stub table names to avoid collisions
// with real tables in the temp schema.
const partitionStubPrefix = "_pgschema_partstub_"

// partitionStubName returns a unique unqualified name for a cross-schema
// partition parent stub: _pgschema_partstub_<schema>__<table>.
func partitionStubName(schema, table string) string {
	return partitionStubPrefix + schema + "__" + table
}

// prependPartitionParentStubs creates stub CREATE TABLE ... PARTITION BY
// statements for cross-schema partition parents referenced by PARTITION OF
// in the desired SQL. Each stub uses a unique prefixed name to avoid
// collisions with real tables or other stubs, and PARTITION OF references
// are rewritten to point to the stub name. Everything stays inside the
// temp schema via search_path.
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
		stubName := partitionStubName(ref.Schema, ref.Table)
		ddl, err := ir.BuildPartitionedTableStubSQL(ctx, conn, ref.Schema, ref.Table, stubName)
		if err != nil {
			return "", err
		}
		if ddl == "" {
			logger.Get().Warn("cross-schema partition parent not found or not partitioned",
				"schema", ref.Schema, "table", ref.Table)
			return "", fmt.Errorf("cross-schema partition parent %s.%s not found or not a partitioned table on the target database", ref.Schema, ref.Table)
		}
		logger.Get().Debug("prepending stub for cross-schema partition parent",
			"schema", ref.Schema, "table", ref.Table, "stubName", stubName)
		stubs.WriteString(ddl)

		// Rewrite PARTITION OF references to point to the stub name.
		rewritten = rewritePartitionOfRef(rewritten, ref.Schema, ref.Table, stubName)
	}

	return stubs.String() + rewritten, nil
}

// rewritePartitionOfRef rewrites "PARTITION OF schema.table" to
// "PARTITION OF stubName" in SQL, handling both quoted and unquoted identifiers.
func rewritePartitionOfRef(sql, schema, table, stubName string) string {
	// Build the qualified reference patterns as they may appear in SQL.
	patterns := []struct{ old, new string }{
		// unquoted: schema.table → stubName
		{
			old: strings.ToLower(schema) + "." + strings.ToLower(table),
			new: ir.QuoteIdentifier(stubName),
		},
	}

	// Also handle quoted identifiers
	quotedSchema := ir.QuoteIdentifier(schema)
	quotedTable := ir.QuoteIdentifier(table)
	quotedStub := ir.QuoteIdentifier(stubName)
	if quotedSchema != schema {
		patterns = append(patterns,
			struct{ old, new string }{quotedSchema + "." + table, quotedStub},
			struct{ old, new string }{quotedSchema + "." + quotedTable, quotedStub},
		)
	}
	if quotedTable != table {
		patterns = append(patterns,
			struct{ old, new string }{schema + "." + quotedTable, quotedStub},
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
		idx := strings.Index(lower[i:], "partition")
		if idx < 0 {
			b.WriteString(sql[i:])
			break
		}
		pos := i + idx
		b.WriteString(sql[i:pos])

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
