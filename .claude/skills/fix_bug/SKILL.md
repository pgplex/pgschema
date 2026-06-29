---
name: fix_bug
description: Fix a bug from a GitHub issue using TDD. Analyzes the issue, creates a reproducing test case, implements the fix, verifies it, and creates a PR. Use this skill whenever working on a GitHub issue, bug report, or regression — even if the user just provides an issue number or URL.
---

# Fix Bug

TDD workflow for fixing bugs from GitHub issues: reproduce first, then fix, then verify.

**Two principles to keep turnaround fast:**

1. **Run only the impacted/relevant tests locally** — never the full suite. CI runs the full suite on the PR; locally you only need to confirm the reproducing test and its immediate neighbors. Narrow `PGSCHEMA_TEST_FILTER` to the specific case (or category) you touched.
2. **Prefer folding `testdata/diff` cases into existing ones** — each test case is its own embedded-postgres apply cycle, so every new directory adds to total test time. By default, consider adding the scenario to a related existing case. Only create a new case when that's more reasonable or consistent with the existing layout (see Phase 2).

## Phase 1: Analyze

1. Fetch the issue: `gh issue view <number>`
2. Classify the bug:
   - **Dump bug**: `pgschema dump` produces wrong output → test in `testdata/dump/`
   - **Diff/Plan bug**: dump is correct but plan generates wrong DDL → test in `testdata/diff/`
   - **Both**: start with dump; if dump is correct, it's a diff bug

## Phase 2: Create Test Case (Red)

### Dump Bugs

Create `testdata/dump/issue_<N>_<description>/` with:
- `manifest.json` — metadata with name, description, source URL, notes
- `raw.sql` — original DDL
- `pgdump.sql` — what pg_dump produces (input to test)
- `pgschema.sql` — expected correct output

Register in `cmd/dump/dump_integration_test.go`:
```go
func TestDumpCommand_Issue<N><Description>(t *testing.T) {
    if testing.Short() { t.Skip("Skipping integration test in short mode") }
    runExactMatchTest(t, "issue_<N>_<description>")
}
```

Verify it fails: `go test -v ./cmd/dump -run TestDumpCommand_Issue<N>`

### Diff/Plan Bugs

Categories: `create_table`, `create_index`, `create_trigger`, `create_view`, `create_function`, `create_procedure`, `create_sequence`, `create_type`, `create_domain`, `create_policy`, `create_materialized_view`, `comment`, `privilege`, `default_privilege`, `dependency`, `online`, `migrate`.

**Decide: fold or create new (default: consider folding).** Each case directory is a separate embedded-postgres apply cycle, so folding keeps the suite fast.

- **Fold** when the bug is a natural variation of an existing case (same object type/category) and adding the DDL doesn't obscure that case's intent. Add the reproducing statements to the existing `old.sql`/`new.sql`, then regenerate its expected outputs. Browse the category first (`ls testdata/diff/<category>/`) to find the best home.
- **Create new** (`testdata/diff/<category>/issue_<N>_<description>/` with `old.sql` and `new.sql`) when the scenario is distinct, or when a standalone `issue_<N>` case is more consistent with how the category is organized.

Verify it fails (use the case name you folded into, or the new `issue_<N>_<description>`):
```bash
PGSCHEMA_TEST_FILTER="<category>/<case>" go test -v ./internal/diff -run TestDiffFromFiles
```

Generate expected outputs once you know correct behavior:
```bash
PGSCHEMA_TEST_FILTER="<category>/<case>" go test -v ./cmd -run TestPlanAndApply --generate
```

## Phase 3: Fix (Green)

Common locations:
- **Dump**: `ir/inspector.go`, `ir/normalize.go`, `internal/dump/`
- **Diff**: `internal/diff/` (`table.go`, `column.go`, `index.go`, `trigger.go`, `view.go`, `function.go`, `procedure.go`, `sequence.go`, `type.go`, `policy.go`, `constraint.go`)
- **IR**: `ir/ir.go`, `ir/quote.go`

Make the minimal fix. Use **pg_dump** and **postgres_syntax** skills as needed.

## Phase 4: Verify

Run only the impacted tests (not the full suite — CI runs that on the PR). Keep the filter as narrow as possible:
```bash
# Dump bugs
go test -v ./cmd/dump -run TestDumpCommand_Issue<N>

# Diff bugs — start with the specific case
PGSCHEMA_TEST_FILTER="<category>/<case>" go test -v ./internal/diff -run TestDiffFromFiles
PGSCHEMA_TEST_FILTER="<category>/<case>" go test -v ./cmd -run TestPlanAndApply
```

Only widen the filter to the whole category (`PGSCHEMA_TEST_FILTER="<category>/"`) if the fix touched shared diff logic that could affect sibling cases.

## Phase 5: Create PR

```bash
git checkout -b fix/issue-<N>-<description>
git add <files>
git commit -m "fix: <description> (#<N>)"
git push -u origin fix/issue-<N>-<description>
gh pr create --title "fix: <description> (#<N>)" --body "## Summary
<what was broken and how it was fixed>

Fixes #<N>

## Test plan
<what test was added and how to run it>"
```

## Checklist

- [ ] Bug classified (dump vs diff)
- [ ] Test case folded into an existing case, or new `issue_<N>_<description>` created when more reasonable
- [ ] Test fails before fix (red)
- [ ] Minimal fix implemented
- [ ] Test passes after fix (green)
- [ ] Impacted tests pass locally (narrow filter — full suite left to CI)
- [ ] PR created and linked to issue
