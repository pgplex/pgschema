# Plan command

Compare a desired-state SQL file with the live database and print migration DDL (human, JSON, and/or SQL).

## Usage

```bash
pgschema plan \
  --host localhost \
  --port 5432 \
  --db mydb \
  --user postgres \
  --schema public \
  --file schema.sql \
  --output-human stdout
```

- **`--file`**: path to the SQL that describes the desired schema (required).
- **`--schema`**: see [Single schema](#single-schema) and [Multi-schema](#multi-schema) below.
- **Outputs**: `--output-human`, `--output-json`, `--output-sql` (each can be `stdout` or a file path). If none are set, human output goes to stdout.

Optional **plan database** (instead of the default embedded Postgres): `--plan-host`, `--plan-port`, `--plan-db`, `--plan-user`, `--plan-password`, `--plan-sslmode`, or `PGSCHEMA_PLAN_*` env vars. See the main project docs for details.

## Single schema

`--schema` defaults to `public`. Only that PostgreSQL namespace is loaded from the target database and from the temporary plan database after your SQL is applied.

## Multi-schema

Pass a **comma-separated** list of schema names (spaces trimmed, duplicates removed):

```bash
pgschema plan \
  --schema public,app \
  --file schema.sql \
  ...
```

### Behaviour

1. **Target (current) state**  
   All listed schemas are introspected and merged into one IR, so the diff can see tables, views, functions, etc. in `public`, `app`, and any other name you include.

2. **Desired state**  
   - The **first** name in the list is the *primary* schema: your `--file` SQL is applied in the temporary plan database with that schema as the strip/normalize target (same as single-schema plan).  
   - After that, the temporary schema **and** every other listed schema are introspected on the plan database. That way, objects you created with explicit qualification (e.g. `app.some_table`) appear in the desired IR as long as `app` is included after the comma.

3. **Generated DDL**  
   Diffing still uses the primary schema for name normalization where applicable; cross-schema references in the IR are preserved as in single-schema mode.

### When to use it

Use multi-schema when a single migration touches more than one namespace (e.g. `public` facts and `app` dimensions) or when foreign keys span schemas you want in the same plan.

### Caveats

- **`dump` / `apply`**: today their `--schema` flag is still a **single** schema name for connection defaults and fingerprinting. A plan built with `--schema public,app` can include DDL for multiple namespaces; applying it may require running `apply` with a workflow that matches your process (for example separate apply runs per schema if you rely on `search_path`), or extending apply in the future.
- **Order matters**: always put the schema where the bulk of unqualified DDL in `--file` lives **first**.

## Running tests

```bash
# All plan tests
go test -v ./cmd/plan/

# Specific plan tests
go test -v ./cmd/plan/ -run "TestPlanCommand_FileToDatabase"
```
