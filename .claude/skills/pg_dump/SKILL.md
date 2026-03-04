---
name: pg_dump
description: Consult PostgreSQL's pg_dump implementation for guidance on system catalog queries and schema extraction when implementing pgschema features. Use this skill when adding new schema object support, debugging inspector.go queries, understanding how PostgreSQL represents objects internally, or handling version-specific features across PostgreSQL 14-18.
---

# pg_dump Reference

Reference pg_dump's implementation for correct system catalog queries and schema extraction patterns.

## Source

**Repository**: https://github.com/postgres/postgres/blob/master/src/bin/pg_dump/

**Key files**:
- `pg_dump.c` - Main implementation with all system catalog queries
- `pg_dump.h` - Data structures
- `pg_dump_sort.c` - Dependency sorting
- `common.c` - Shared catalog query utilities

## Object Type → pg_dump Function → System Catalogs

| Object | Function | Catalogs |
|--------|----------|----------|
| Tables & Columns | `getTables()` | `pg_class`, `pg_attribute`, `pg_type` |
| Constraints | `getConstraints()` | `pg_constraint` |
| Indexes | `getIndexes()` | `pg_index`, `pg_class` |
| Triggers | `getTriggers()` | `pg_trigger`, `pg_proc` |
| Functions | `getFuncs()` | `pg_proc` |
| Views | `getViews()` | `pg_class`, `pg_rewrite` |
| Sequences | `getSequences()` | `pg_sequence`, `pg_class` |
| Policies | `getPolicies()` | `pg_policy` |
| Types | `getTypes()` | `pg_type` |
| Aggregates | `getAggregates()` | `pg_aggregate`, `pg_proc` |
| Comments | `getComments()` | `pg_description` |

## Key Helper Functions

- `pg_get_expr(expr, relation, pretty)` - Deparse expressions (defaults, WHEN clauses, index predicates)
- `pg_get_constraintdef(oid, pretty)` - Get constraint DDL
- `pg_get_indexdef(oid, column, pretty)` - Get index DDL
- `pg_get_triggerdef(oid, pretty)` - Get full trigger DDL

**Important**: For trigger WHEN clauses, always use `pg_get_expr(t.tgqual, t.tgrelid, false)` from `pg_catalog.pg_trigger`. Do NOT use `information_schema.triggers.action_condition`.

## Workflow

1. **Identify the object type** you're implementing
2. **Find the pg_dump function** from the table above
3. **Read the system catalog query** — note which columns, joins, and helper functions are used
4. **Check version-specific handling** — pg_dump uses `fout->remoteVersion` checks
5. **Adapt for pgschema** in `ir/inspector.go` or `ir/queries/`:
   - Use pgx parameter binding
   - Handle NULLs appropriately
   - Add version detection if needed

## When pg_dump is Authoritative

Always reference for: system catalog query patterns, correct use of `pg_get_*` functions, version-specific feature detection, object dependency tracking.

## When NOT to Copy pg_dump

Don't copy: output formatting (pgschema has different conventions), archive/restore logic, full-database scope (pgschema is schema-focused), pre-PG14 compatibility.

## pgschema Adaptation Pattern

```go
// pg_dump query pattern:
// SELECT t.tgname, pg_get_expr(t.tgqual, t.tgrelid, false) as when_clause
// FROM pg_catalog.pg_trigger t WHERE ...

// pgschema adaptation (in ir/inspector.go or ir/queries/):
query := `SELECT t.tgname, pg_get_expr(t.tgqual, t.tgrelid, false)
FROM pg_catalog.pg_trigger t
WHERE t.tgrelid = $1 AND NOT t.tgisinternal`
rows, err := conn.Query(ctx, query, tableOID)
```
