---
name: run_tests
description: Run pgschema automated tests (go test) to validate diff logic, plan generation, and dump functionality using test fixtures. Use this skill whenever you need to run tests, debug test failures, add new test cases, regenerate expected outputs, or validate changes across PostgreSQL versions 14-18.
---

# Run Tests

Run pgschema tests for validating implementation changes.

## Test Types

| Type | Command | Speed | What it tests |
|------|---------|-------|---------------|
| Diff | `go test -v ./internal/diff -run TestDiffFromFiles` | ~2s | Schema comparison logic (no DB) |
| Plan/Apply | `go test -v ./cmd -run TestPlanAndApply` | ~30-60s | Full workflow with embedded PostgreSQL |
| Dump | `go test -v ./cmd/dump -run TestDumpCommand` | ~10-20s | Schema extraction from test databases |

## Filtering Tests

Use `PGSCHEMA_TEST_FILTER` to scope to specific cases:

```bash
# Specific test case
PGSCHEMA_TEST_FILTER="create_trigger/add_trigger_when_distinct" go test -v ./internal/diff -run TestDiffFromFiles

# All tests in a category
PGSCHEMA_TEST_FILTER="create_trigger/" go test -v ./cmd -run TestPlanAndApply
```

**Categories** (in `testdata/diff/`): `comment/`, `create_domain/`, `create_function/`, `create_index/`, `create_materialized_view/`, `create_policy/`, `create_procedure/`, `create_sequence/`, `create_table/`, `create_trigger/`, `create_type/`, `create_view/`, `default_privilege/`, `privilege/`, `dependency/`, `online/`, `migrate/`

## Regenerating Expected Output

When implementation intentionally changes generated DDL:

```bash
PGSCHEMA_TEST_FILTER="category/test_name" go test -v ./cmd -run TestPlanAndApply --generate
```

This overwrites `diff.sql`, `plan.json`, `plan.sql`, `plan.txt`. Always review with `git diff` afterward and re-run without `--generate` to confirm.

## Testing Across PostgreSQL Versions

```bash
PGSCHEMA_POSTGRES_VERSION=14 go test -v ./cmd/dump -run TestDumpCommand_Employee
PGSCHEMA_POSTGRES_VERSION=17 PGSCHEMA_TEST_FILTER="create_trigger/" go test -v ./cmd -run TestPlanAndApply
```

Supported: 14, 15, 16, 17, 18.

## Adding a New Test Case

1. Create directory: `mkdir -p testdata/diff/<category>/<test_name>`
2. Create `old.sql` (starting state) and `new.sql` (desired state)
3. Generate expected files: `PGSCHEMA_TEST_FILTER="<category>/<test_name>" go test -v ./cmd -run TestPlanAndApply --generate`
4. Review generated `diff.sql` and plan files
5. Run without `--generate` to verify

For dump tests, create `testdata/dump/<name>/` with `manifest.json`, `raw.sql`, `pgdump.sql`, `pgschema.sql` and register in `cmd/dump/dump_integration_test.go`.

## Test Data Structure

```
testdata/diff/<category>/<test_name>/
├── old.sql       # Starting schema
├── new.sql       # Desired schema
├── diff.sql      # Expected migration DDL
├── plan.json     # Plan in JSON
├── plan.sql      # Plan as SQL
└── plan.txt      # Plan as text
```

## Recommended Testing Order

1. **Fast iteration**: diff tests first (no DB startup)
2. **Full validation**: integration tests for the same category
3. **Before commit**: `go test -v ./...` (or scoped to affected areas if full suite times out)

## Environment Variables

| Variable | Purpose | Example |
|----------|---------|---------|
| `PGSCHEMA_TEST_FILTER` | Run specific test cases | `"create_trigger/"` |
| `PGSCHEMA_POSTGRES_VERSION` | Test specific PG version | `14`, `17` |
