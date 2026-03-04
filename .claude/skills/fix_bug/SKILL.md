---
name: fix_bug
description: Fix a bug from a GitHub issue using TDD. Analyzes the issue, creates a reproducing test case, implements the fix, verifies it, and creates a PR. Use this skill whenever working on a GitHub issue, bug report, or regression — even if the user just provides an issue number or URL.
---

# Fix Bug

TDD workflow for fixing bugs from GitHub issues: reproduce first, then fix, then verify.

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

Create `testdata/diff/<category>/issue_<N>_<description>/` with `old.sql` and `new.sql`.

Categories: `create_table`, `create_index`, `create_trigger`, `create_view`, `create_function`, `create_procedure`, `create_sequence`, `create_type`, `create_domain`, `create_policy`, `create_materialized_view`, `comment`, `privilege`, `default_privilege`, `dependency`, `online`, `migrate`.

Verify it fails:
```bash
PGSCHEMA_TEST_FILTER="<category>/issue_<N>_<description>" go test -v ./internal/diff -run TestDiffFromFiles
```

Generate expected outputs once you know correct behavior:
```bash
PGSCHEMA_TEST_FILTER="<category>/issue_<N>_<description>" go test -v ./cmd -run TestPlanAndApply --generate
```

## Phase 3: Fix (Green)

Common locations:
- **Dump**: `ir/inspector.go`, `ir/normalize.go`, `internal/dump/`
- **Diff**: `internal/diff/` (`table.go`, `column.go`, `index.go`, `trigger.go`, `view.go`, `function.go`, `procedure.go`, `sequence.go`, `type.go`, `policy.go`, `constraint.go`)
- **IR**: `ir/ir.go`, `ir/quote.go`

Make the minimal fix. Use **pg_dump** and **postgres_syntax** skills as needed.

## Phase 4: Verify

Run targeted tests (not the full suite — that runs in CI):
```bash
# Dump bugs
go test -v ./cmd/dump -run TestDumpCommand_Issue<N>

# Diff bugs
PGSCHEMA_TEST_FILTER="<category>/issue_<N>" go test -v ./internal/diff -run TestDiffFromFiles
PGSCHEMA_TEST_FILTER="<category>/" go test -v ./cmd -run TestPlanAndApply
```

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
- [ ] Test case created (`issue_<N>_<description>`)
- [ ] Test fails before fix (red)
- [ ] Minimal fix implemented
- [ ] Test passes after fix (green)
- [ ] Related tests pass (no regressions)
- [ ] PR created and linked to issue
