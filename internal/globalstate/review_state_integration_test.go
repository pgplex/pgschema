package globalstate

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

func TestReviewRoutineOwnershipResolvesNamedIdentityArguments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, target)
	defer db.Close()
	major := reviewServerMajor(t, db)
	_, err := db.Exec(`CREATE FUNCTION public.calculate(value integer) RETURNS integer LANGUAGE sql AS 'SELECT value'`)
	require.NoError(t, err)

	selection := Selection{Ownership: []Ownership{{Kind: "function", Name: "public.calculate(integer)", Owner: "postgres"}}}
	snapshot, err := Inspect(context.Background(), db, selection, major)
	require.NoError(t, err)
	state, exists := snapshot.Ownership[ownershipKey("function", "public.calculate(integer)")]
	require.True(t, exists)
	require.Equal(t, snapshot.SessionRole.Name, state.Owner)
}

func TestReviewRoleDependencyInspectionFindsOwnedObjects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, target)
	defer db.Close()
	major := reviewServerMajor(t, db)
	_, err := db.Exec(`CREATE ROLE retired; CREATE TABLE public.retained (id integer); ALTER TABLE public.retained OWNER TO retired`)
	require.NoError(t, err)

	selection := Selection{Roles: []string{"retired"}}
	snapshot, err := Inspect(context.Background(), db, selection, major)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.RoleDependencyCounts["retired"])
}

func TestReviewRoleDependencyInspectionIgnoresMembershipsDroppedWithRole(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, target)
	defer db.Close()
	major := reviewServerMajor(t, db)
	_, err := db.Exec(`CREATE ROLE app_group; CREATE ROLE retired; GRANT app_group TO retired`)
	require.NoError(t, err)

	snapshot, err := Inspect(context.Background(), db, Selection{Roles: []string{"retired"}}, major)
	require.NoError(t, err)
	require.Zero(t, snapshot.RoleDependencyCounts["retired"])
}

func reviewServerMajor(t *testing.T, db interface {
	QueryRow(query string, args ...any) *sql.Row
}) int {
	t.Helper()
	var version int
	require.NoError(t, db.QueryRow("SHOW server_version_num").Scan(&version))
	return version / 10000
}
