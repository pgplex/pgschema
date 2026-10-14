package plan

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/ir"
	"github.com/stretchr/testify/require"
)

func TestIndexRecoveryIsolation(t *testing.T) {
	index := &ir.Index{Schema: "Quoted.Schema", Table: "table", Name: "idx'$pgschema$"}
	p := NewPlan([]diff.Diff{{Type: diff.DiffTypeTableIndex, Operation: diff.DiffOperationAlter, Source: &diff.IndexRecovery{Index: index}}, {Type: diff.DiffTypeTable, Operation: diff.DiffOperationAlter, Statements: []diff.SQLStatement{{SQL: "SELECT 1;", CanRunInTransaction: true}}}}, 18, nil)
	require.Len(t, p.Groups, 4, "precondition, concurrent rebuild, postcondition and following DDL must be separate")
	for _, g := range p.Groups {
		require.Len(t, g.Steps, 1)
	}
	require.Equal(t, `REINDEX INDEX CONCURRENTLY "Quoted.Schema"."idx'$pgschema$";`, p.Groups[1].Steps[0].SQL)
	require.True(t, strings.HasPrefix(p.Groups[0].Steps[0].SQL, "DO $pgschema_$"), "legal names must not terminate the DO body")
	require.Contains(t, p.Groups[0].Steps[0].SQL, "idx''$pgschema$")
	require.Contains(t, p.Groups[2].Steps[0].SQL, "RAISE EXCEPTION")
}
