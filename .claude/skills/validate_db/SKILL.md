---
name: validate_db
description: Connect to live PostgreSQL database to validate schema assumptions, compare pg_dump vs pgschema output, and query system catalogs interactively. Use this skill whenever you need to test queries against a real database, verify how PostgreSQL stores or formats objects, debug introspection issues, or check version-specific behavior (PostgreSQL 14-18).
---

# Validate with Database

Connect to the test PostgreSQL database to validate assumptions, compare implementations, and test queries.

## Connection

Default test credentials: password `testpwd1`, user `postgres`, host `localhost`, port `5432`. Also see `.env` at project root.

```bash
# One-off query
PGPASSWORD='testpwd1' psql -h localhost -p 5432 -U postgres -d employee -c "SELECT version();"

# pg_dump schema
PGPASSWORD='testpwd1' pg_dump -h localhost -p 5432 -U postgres -d employee --schema-only --schema=public

# pgschema dump (reads .env automatically)
./pgschema dump --schema public
```

## Core Workflows

### Compare pg_dump vs pgschema

```bash
# 1. Dump both
PGPASSWORD='testpwd1' pg_dump -h localhost -p 5432 -U postgres -d employee --schema-only --schema=public > /tmp/pg_dump_output.sql
./pgschema dump --schema public -o /tmp/pgschema_output.sql

# 2. Compare
diff -u /tmp/pg_dump_output.sql /tmp/pgschema_output.sql
```

Look for: missing objects (bugs), different DDL structure (investigate), formatting differences (expected).

### Validate System Catalog Queries

1. Create test objects in a test database
2. Query system catalogs to verify expected data
3. Compare with pgschema output
4. Clean up test objects

### Test Plan/Apply Workflow

```bash
# 1. Create test schema
PGPASSWORD='testpwd1' psql -h localhost -p 5432 -U postgres -d postgres -c "
DROP SCHEMA IF EXISTS test_workflow CASCADE;
CREATE SCHEMA test_workflow;
CREATE TABLE test_workflow.users (id SERIAL PRIMARY KEY, email TEXT NOT NULL UNIQUE);
"

# 2. Dump, modify, plan, apply
./pgschema dump --schema test_workflow -o /tmp/schema.sql
# Edit /tmp/schema.sql with desired changes
./pgschema plan --schema test_workflow --file /tmp/schema.sql
./pgschema apply --schema test_workflow --file /tmp/schema.sql --auto-approve

# 3. Verify and cleanup
PGPASSWORD='testpwd1' psql -h localhost -p 5432 -U postgres -d postgres -c "\d test_workflow.users"
PGPASSWORD='testpwd1' psql -h localhost -p 5432 -U postgres -d postgres -c "DROP SCHEMA IF EXISTS test_workflow CASCADE;"
```

### Cross-Version Testing

```bash
PGSCHEMA_POSTGRES_VERSION=14 go test -v ./cmd/dump -run TestDumpCommand_Employee
PGSCHEMA_POSTGRES_VERSION=17 go test -v ./cmd/dump -run TestDumpCommand_Employee
```

Supported versions: 14, 15, 16, 17, 18.

## Useful System Catalog Queries

```sql
-- Columns with types
SELECT a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod) as data_type,
       a.attnotnull, pg_get_expr(ad.adbin, ad.adrelid) as default_value, a.attgenerated
FROM pg_attribute a
LEFT JOIN pg_attrdef ad ON (a.attrelid = ad.adrelid AND a.attnum = ad.adnum)
WHERE a.attrelid = 'public.TABLE_NAME'::regclass AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum;

-- Constraints
SELECT con.conname, con.contype, pg_get_constraintdef(con.oid)
FROM pg_constraint con WHERE con.conrelid = 'public.TABLE_NAME'::regclass;

-- Indexes
SELECT i.relname, am.amname, pg_get_indexdef(idx.indexrelid),
       CASE WHEN idx.indpred IS NOT NULL THEN pg_get_expr(idx.indpred, idx.indrelid, true) END as where_clause
FROM pg_index idx
JOIN pg_class i ON i.oid = idx.indexrelid
JOIN pg_class t ON t.oid = idx.indrelid
JOIN pg_am am ON i.relam = am.oid
WHERE t.relname = 'TABLE_NAME' AND t.relnamespace = 'public'::regnamespace;

-- Triggers
SELECT t.tgname, pg_get_triggerdef(t.oid),
       CASE WHEN t.tgqual IS NOT NULL THEN pg_get_expr(t.tgqual, t.tgrelid, false) END as when_condition
FROM pg_trigger t
JOIN pg_class c ON t.tgrelid = c.oid
WHERE c.relname = 'TABLE_NAME' AND c.relnamespace = 'public'::regnamespace AND NOT t.tgisinternal;

-- Comments on tables
SELECT c.relname, d.description FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_description d ON d.objoid = c.oid AND d.objsubid = 0
WHERE n.nspname = 'public' AND c.relkind = 'r';
```

## Checklist

- [ ] Database is running and accessible
- [ ] pg_dump produces expected output
- [ ] pgschema produces comparable output
- [ ] System catalog queries return expected data
- [ ] Plan generates correct migration DDL
- [ ] Tested across PostgreSQL versions if version-specific
- [ ] Test database cleaned up
