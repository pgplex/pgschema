package ir

import (
	"context"
	"fmt"
)

// IndexState records operational state separately from the index definition.
// PostgreSQL can retain an invalid index after an interrupted concurrent build,
// including one that is ready for writes and still enforces uniqueness.
type IndexState struct {
	Table      string `json:"table"`
	IndexKind  string `json:"index_kind"`
	TableKind  string `json:"table_kind"`
	Valid      bool   `json:"valid"`
	Ready      bool   `json:"ready"`
	Live       bool   `json:"live"`
	Constraint string `json:"constraint,omitempty"`
	// Progress and locks are transient observations, not schema fingerprints.
	BuildInProgress      bool `json:"-"`
	ConflictingOperation bool `json:"-"`
}

func (i *Inspector) buildIndexStates(ctx context.Context, result *IR, targetSchema string) error {
	rows, err := i.queries.GetUnhealthyIndexesForSchema(ctx, targetSchema)
	if err != nil {
		return err
	}
	schema := result.Schemas[targetSchema]
	if schema == nil {
		return nil
	}
	for _, row := range rows {
		if i.ignoreConfig != nil && (i.ignoreConfig.ShouldIgnoreIndex(row.IndexName) || (row.ConstraintName != "" && i.ignoreConfig.ShouldIgnoreConstraint(row.ConstraintName))) {
			continue
		}
		// The regular inspector already applies table/view/extension ignore rules.
		if schema.Tables[row.TableName] == nil && schema.Views[row.TableName] == nil {
			continue
		}
		if schema.IndexStates == nil {
			schema.IndexStates = make(map[string]*IndexState)
		}
		schema.IndexStates[row.IndexName] = &IndexState{
			Table: row.TableName, IndexKind: row.IndexKind, TableKind: row.TableKind,
			Valid: row.IsValid, Ready: row.IsReady, Live: row.IsLive, Constraint: row.ConstraintName,
			BuildInProgress: row.BuildInProgress, ConflictingOperation: row.ConflictingOperation,
		}
	}
	return nil
}

// CheckOperation refuses to mutate an unhealthy index while its table has an
// in-flight build or conflicting maintenance. Call only for affected indexes:
// an intentional ON ONLY partition parent must not block unrelated plans.
func (s *IndexState) CheckOperation(schema, name string) error {
	if s.BuildInProgress || s.ConflictingOperation {
		return fmt.Errorf("index %s.%s is not usable while an index build or conflicting maintenance operation is active; wait for it to finish, then regenerate the plan", QuoteIdentifier(schema), QuoteIdentifier(name))
	}
	return nil
}
