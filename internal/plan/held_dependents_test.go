package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/testutil"
)

// TestHeldDependentsShareOneTransaction checks, for every diff test case, that
// the plan puts no transaction boundary between the first and the last step
// that drops or restores an object held around a function that is dropped and
// created again: the drops, the recreation and the restores commit together,
// so the objects are never missing between transactions, even when the same
// plan has steps of other objects that need transactions of their own (#601).
func TestHeldDependentsShareOneTransaction(t *testing.T) {
	root := "../../testdata/diff"
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, sharedTestPostgres)
	majorVersion, err := testutil.GetMajorVersion(conn)
	conn.Close()
	if err != nil {
		t.Fatalf("Failed to detect PostgreSQL version: %v", err)
	}

	casesWithHeldDependents := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return err
		}
		oldSQL, errOld := os.ReadFile(filepath.Join(path, "old.sql"))
		newSQL, errNew := os.ReadFile(filepath.Join(path, "new.sql"))
		if errOld != nil || errNew != nil {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		name := strings.ReplaceAll(relPath, string(os.PathSeparator), "_")
		t.Run(name, func(t *testing.T) {
			testutil.ShouldSkipTest(t, name, majorVersion)
			setupSQL, _ := os.ReadFile(filepath.Join(path, "setup.sql"))
			oldIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, string(oldSQL), "public", string(setupSQL))
			newIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, string(newSQL), "public", string(setupSQL))
			groups := groupDiffs(diff.GenerateMigrationForTarget(oldIR, newIR, "public", majorVersion), majorVersion, oldIR)

			first, last := -1, -1
			for i, group := range groups {
				for _, step := range group.Steps {
					if step.heldDependent {
						if first < 0 {
							first = i
						}
						last = i
					}
				}
			}
			if first < 0 {
				return
			}
			casesWithHeldDependents++
			if first != last {
				t.Errorf("held drops and restores span transaction groups %d to %d", first+1, last+1)
			}
			// Nor may any other statement change a held object outside that
			// transaction, e.g. a DROP DEFAULT of a held column emitted by the
			// regular column diff. Only the VALIDATE CONSTRAINT of a CHECK
			// constraint re-added NOT VALID follows in a transaction of its own.
			for i, group := range groups {
				for _, step := range group.Steps {
					if i > first && strings.Contains(step.SQL, " VALIDATE CONSTRAINT ") {
						continue
					}
					if !step.heldDependent && i != first && touchesHeldObject(step, groups) {
						t.Errorf("statement on a held object in transaction group %d, outside the held span (group %d): %s", i+1, first+1, step.SQL)
					}
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk %s: %v", root, err)
	}
	if casesWithHeldDependents == 0 && majorVersion == testutil.LatestPostgresVersion {
		t.Errorf("no test case has held dependents; the check ran on nothing")
	}
}

// touchesHeldObject reports whether an unmarked step changes an object that
// held steps drop or restore: the same constraint, index, policy or trigger,
// or the default of the same column or domain.
func touchesHeldObject(step Step, groups []ExecutionGroup) bool {
	for _, group := range groups {
		for _, held := range group.Steps {
			if !held.heldDependent || held.Type != step.Type || held.Path != step.Path {
				continue
			}
			switch step.Type {
			case "table.column", "domain":
				if strings.Contains(step.SQL, " DEFAULT") && strings.Contains(held.SQL, " DEFAULT") {
					return true
				}
			default:
				return true
			}
		}
	}
	return false
}
