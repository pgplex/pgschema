package fingerprint

import (
	"encoding/json"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/stretchr/testify/require"
)

func TestIndexStateFingerprint(t *testing.T) {
	schema := &ir.Schema{Name: "public"}
	state := &ir.IR{Schemas: map[string]*ir.Schema{"public": schema}}
	healthy, err := ComputeFingerprint(state, "public")
	require.NoError(t, err)
	original, err := json.Marshal(schema)
	require.NoError(t, err)
	require.NotContains(t, string(original), "index_states")
	schema.IndexStates = map[string]*ir.IndexState{}
	same, err := ComputeFingerprint(state, "public")
	require.NoError(t, err)
	require.Equal(t, healthy, same, "empty health metadata must preserve pre-fix fingerprints")
	schema.IndexStates["idx"] = &ir.IndexState{Table: "t", IndexKind: "i", TableKind: "r", Live: true}
	invalid, err := ComputeFingerprint(state, "public")
	require.NoError(t, err)
	require.NotEqual(t, healthy, invalid)
	schema.IndexStates["idx"].BuildInProgress = true
	active, err := ComputeFingerprint(state, "public")
	require.NoError(t, err)
	require.Equal(t, invalid, active, "PIDs/progress must not create schema drift")
	schema.IndexStates["idx"].Ready = true
	ready, err := ComputeFingerprint(state, "public")
	require.NoError(t, err)
	require.NotEqual(t, invalid, ready)
}
