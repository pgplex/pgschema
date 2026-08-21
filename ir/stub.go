package ir

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// BuildTableStubSQL returns CREATE SCHEMA / CREATE TABLE DDL that is sufficient
// for foreign keys to reference schema.table: all columns plus PRIMARY KEY and
// UNIQUE constraints. Defaults, identity, generated expressions, and foreign
// keys on the ignored table itself are omitted.
//
// Returns an empty string if the table does not exist.
func BuildTableStubSQL(ctx context.Context, db *sql.DB, schema, table, targetSchema string) (string, error) {
	cols, err := queryStubColumns(ctx, db, schema, table)
	if err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", nil
	}

	constraints, err := queryStubConstraints(ctx, db, schema, table)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	qualified := QualifyEntityNameWithQuotesMode(schema, table, targetSchema, schema != targetSchema)

	if schema != targetSchema {
		b.WriteString("CREATE SCHEMA IF NOT EXISTS ")
		b.WriteString(QuoteIdentifier(schema))
		b.WriteString(";\n")
	}

	b.WriteString("-- pgschema: stub for ignored table ")
	b.WriteString(sanitizeComment(schema))
	b.WriteString(".")
	b.WriteString(sanitizeComment(table))
	b.WriteString("\nCREATE TABLE IF NOT EXISTS ")
	b.WriteString(qualified)
	b.WriteString(" (\n")

	for i, col := range cols {
		b.WriteString("    ")
		b.WriteString(QuoteIdentifier(col.name))
		b.WriteString(" ")
		b.WriteString(col.dataType)
		if col.notNull {
			b.WriteString(" NOT NULL")
		}
		if i < len(cols)-1 || len(constraints) > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}

	for i, def := range constraints {
		b.WriteString("    ")
		b.WriteString(def)
		if i < len(constraints)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}

	b.WriteString(");\n")
	return b.String(), nil
}

// BuildPartitionedTableStubSQL returns CREATE SCHEMA / CREATE TABLE DDL for a
// partitioned parent table that is referenced by PARTITION OF from a child in
// another schema. The stub includes the PARTITION BY clause so the child can
// attach as a partition.
//
// Returns an empty string if the table does not exist or is not partitioned.
func BuildPartitionedTableStubSQL(ctx context.Context, db *sql.DB, schema, table, targetSchema string) (string, error) {
	cols, err := queryStubColumns(ctx, db, schema, table)
	if err != nil {
		return "", err
	}
	if len(cols) == 0 {
		return "", nil
	}

	partDef, err := queryPartitionKeyDef(ctx, db, schema, table)
	if err != nil {
		return "", err
	}
	if partDef == "" {
		return "", nil
	}

	constraints, err := queryStubConstraints(ctx, db, schema, table)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	qualified := QualifyEntityNameWithQuotesMode(schema, table, targetSchema, schema != targetSchema)

	if schema != targetSchema {
		b.WriteString("CREATE SCHEMA IF NOT EXISTS ")
		b.WriteString(QuoteIdentifier(schema))
		b.WriteString(";\n")
	}

	b.WriteString("-- pgschema: stub for cross-schema partition parent ")
	b.WriteString(sanitizeComment(schema))
	b.WriteString(".")
	b.WriteString(sanitizeComment(table))
	b.WriteString("\nCREATE TABLE IF NOT EXISTS ")
	b.WriteString(qualified)
	b.WriteString(" (\n")

	for i, col := range cols {
		b.WriteString("    ")
		b.WriteString(QuoteIdentifier(col.name))
		b.WriteString(" ")
		b.WriteString(col.dataType)
		if col.notNull {
			b.WriteString(" NOT NULL")
		}
		if i < len(cols)-1 || len(constraints) > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}

	for i, def := range constraints {
		b.WriteString("    ")
		b.WriteString(def)
		if i < len(constraints)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}

	b.WriteString(") PARTITION BY ")
	b.WriteString(partDef)
	b.WriteString(";\n")
	return b.String(), nil
}

func queryPartitionKeyDef(ctx context.Context, db *sql.DB, schema, table string) (string, error) {
	const q = `
SELECT pg_catalog.pg_get_partkeydef(c.oid)
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relname = $2
  AND c.relkind = 'p'`

	var def string
	err := db.QueryRowContext(ctx, q, schema, table).Scan(&def)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("query partition key def for %s.%s: %w", schema, table, err)
	}
	return def, nil
}

type stubColumn struct {
	name     string
	dataType string
	notNull  bool
}

func queryStubColumns(ctx context.Context, db *sql.DB, schema, table string) ([]stubColumn, error) {
	const q = `
SELECT
    a.attname,
    pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
    a.attnotnull
FROM pg_catalog.pg_attribute a
JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relname = $2
  AND c.relkind IN ('r', 'p')
  AND a.attnum > 0
  AND NOT a.attisdropped
ORDER BY a.attnum`

	rows, err := db.QueryContext(ctx, q, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query columns for %s.%s: %w", schema, table, err)
	}
	defer rows.Close()

	var cols []stubColumn
	for rows.Next() {
		var col stubColumn
		if err := rows.Scan(&col.name, &col.dataType, &col.notNull); err != nil {
			return nil, fmt.Errorf("scan columns for %s.%s: %w", schema, table, err)
		}
		cols = append(cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

func queryStubConstraints(ctx context.Context, db *sql.DB, schema, table string) ([]string, error) {
	const q = `
SELECT pg_catalog.pg_get_constraintdef(con.oid, true)
FROM pg_catalog.pg_constraint con
JOIN pg_catalog.pg_class c ON c.oid = con.conrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relname = $2
  AND con.contype IN ('p', 'u')
ORDER BY con.contype, con.conname`

	rows, err := db.QueryContext(ctx, q, schema, table)
	if err != nil {
		return nil, fmt.Errorf("query constraints for %s.%s: %w", schema, table, err)
	}
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def string
		if err := rows.Scan(&def); err != nil {
			return nil, fmt.Errorf("scan constraints for %s.%s: %w", schema, table, err)
		}
		if def != "" {
			defs = append(defs, def)
		}
	}
	return defs, rows.Err()
}

// sanitizeComment replaces control characters (newlines, tabs, etc.) in a
// string destined for a SQL line comment so quoted identifiers cannot break
// out of the comment and inject SQL.
func sanitizeComment(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r < 0x20 {
			return ' '
		}
		return r
	}, s)
}
