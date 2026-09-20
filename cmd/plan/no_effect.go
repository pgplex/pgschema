package plan

import (
	"fmt"
	"io"
	"os"

	"github.com/pgplex/pgschema/internal/postgres"
)

// noEffectReasons explains, per kind, why the statement cannot change the plan.
var noEffectReasons = map[postgres.NoEffectKind]string{
	postgres.NoEffectOwner:                   "pgschema does not manage object ownership; change the owner outside pgschema, see https://www.pgschema.com/syntax/unsupported#object-ownership",
	postgres.NoEffectGlobalDefaultPrivileges: "pgschema manages default privileges only with IN SCHEMA; apply global ones outside pgschema, see https://www.pgschema.com/syntax/unsupported#global-default-privileges",
}

// warningWriter receives user-facing plan warnings; tests replace it.
var warningWriter io.Writer = os.Stderr

// stripNoEffectStatements removes desired-state statements that cannot change
// the plan and warns about each on w. Without the warning such a statement
// yields a successful empty plan, indistinguishable from convergence
// (issues #602, #603).
func stripNoEffectStatements(w io.Writer, desiredSQL string) string {
	stripped, found := postgres.StripNoEffectStatements(desiredSQL)
	for _, stmt := range found {
		fmt.Fprintf(w, "Warning: statement has no effect: %s\n  %s\n", stmt.SQL, noEffectReasons[stmt.Kind])
	}
	return stripped
}
