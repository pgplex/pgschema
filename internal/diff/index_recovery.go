package diff

import (
	"fmt"
	"sort"

	"github.com/pgplex/pgschema/ir"
)

// IndexRecovery distinguishes repair from a definition change: the online
// replacement rewrite must never drop an invalid index before rebuilding it.
type IndexRecovery struct{ Index *ir.Index }

func (r *IndexRecovery) GetObjectName() string { return r.Index.Name }

// IndexRecoveryDiffs validates abnormal catalog states and returns repairs to
// execute before ordinary migration steps. PostgreSQL owns the replacement and
// dependency transfer; failed unique builds remain failures, never no-ops.
func IndexRecoveryDiffs(current, desired *ir.IR, schemaName string) ([]Diff, error) {
	oldSchema, newSchema := current.Schemas[schemaName], desired.Schemas[schemaName]
	if oldSchema == nil {
		return nil, nil
	}
	names := make([]string, 0, len(oldSchema.IndexStates))
	for name := range oldSchema.IndexStates {
		names = append(names, name)
	}
	sort.Strings(names)
	var repairs []Diff
	for _, name := range names {
		state := oldSchema.IndexStates[name]
		qualified := ir.QuoteIdentifier(schemaName) + "." + ir.QuoteIdentifier(name)
		var wantedState *ir.IndexState
		if newSchema != nil {
			wantedState = newSchema.IndexStates[name]
		}
		// Explicit constraint removal is an ordinary dependency-aware migration,
		// including an intentionally incomplete partitioned constraint.
		if state.Constraint != "" && (newSchema == nil || newSchema.Tables[state.Table] == nil || newSchema.Tables[state.Table].Constraints[state.Constraint] == nil) {
			if err := state.CheckOperation(schemaName, name); err != nil {
				return nil, err
			}
			continue
		}
		if state.IndexKind == "I" {
			// CREATE INDEX ON ONLY intentionally leaves a partitioned parent invalid.
			// Attaching all matching leaf indexes, not REINDEX, validates that parent.
			if wantedState != nil && wantedState.IndexKind == "I" && state.Table == wantedState.Table && state.Valid == wantedState.Valid && state.Ready == wantedState.Ready && state.Live == wantedState.Live && state.Constraint == wantedState.Constraint {
				continue
			}
			if err := state.CheckOperation(schemaName, name); err != nil {
				return nil, err
			}
			// An intentionally incomplete ordinary parent may be removed along
			// with its index or table through the usual dependency-aware diff.
			if state.Constraint == "" && findRecoveryIndex(newSchema, state.Table, name) == nil {
				continue
			}
			return nil, fmt.Errorf("partitioned index %s is invalid; create and attach the missing partition indexes before replanning (REINDEX cannot complete partition attachment)", qualified)
		}
		if err := state.CheckOperation(schemaName, name); err != nil {
			return nil, err
		}
		if state.Constraint != "" {
			return nil, fmt.Errorf("constraint-backed index %s is not usable; inspect constraint %s and repair its index explicitly before replanning; automatic recovery will not drop integrity constraints", qualified, ir.QuoteIdentifier(state.Constraint))
		}
		oldIndex := findRecoveryIndex(oldSchema, state.Table, name)
		newIndex := findRecoveryIndex(newSchema, state.Table, name)
		// Undesired physical indexes retain normal DROP semantics. This also permits
		// cleanup of abandoned concurrent REINDEX artifacts, without name heuristics.
		if newIndex == nil {
			continue
		}
		if !state.Live || state.Valid || state.IndexKind != "i" || (state.TableKind != "r" && state.TableKind != "m") {
			return nil, fmt.Errorf("index %s has unsupported catalog state (valid=%t, ready=%t, live=%t); finish or repair the interrupted operation before replanning", qualified, state.Valid, state.Ready, state.Live)
		}
		if wantedState != nil {
			return nil, fmt.Errorf("desired index %s is not usable in the planning database; check the schema file and planning database before regenerating the plan", qualified)
		}
		if oldIndex == nil || !indexesStructurallyEqual(oldIndex, newIndex) {
			return nil, fmt.Errorf("invalid index %s also has a requested definition change; recover its original definition first or use an explicitly authored migration, then regenerate the plan", qualified)
		}
		kind := DiffTypeTableIndex
		if state.TableKind == "m" {
			kind = DiffTypeMaterializedViewIndex
		}
		repairs = append(repairs, Diff{
			Type: kind, Operation: DiffOperationAlter, Path: schemaName + "." + state.Table + "." + name,
			Source:     &IndexRecovery{Index: oldIndex},
			Statements: []SQLStatement{{SQL: "REINDEX INDEX CONCURRENTLY " + qualified + ";", CanRunInTransaction: false}},
		})
	}
	return repairs, nil
}

func findRecoveryIndex(schema *ir.Schema, table, name string) *ir.Index {
	if schema == nil {
		return nil
	}
	if t := schema.Tables[table]; t != nil {
		return t.Indexes[name]
	}
	if v := schema.Views[table]; v != nil {
		return v.Indexes[name]
	}
	return nil
}
