package apply

import (
	"context"
	"testing"

	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

func TestReviewExecutionRoleSupportsConcurrentIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	db, _, _, _, _, _ := testutil.ConnectToPostgres(t, target)
	defer db.Close()
	_, err := db.Exec(`
CREATE ROLE app_owner;
CREATE TABLE public.documents (id integer);
ALTER TABLE public.documents OWNER TO app_owner;
GRANT CREATE ON SCHEMA public TO app_owner`)
	require.NoError(t, err)
	group := plan.ExecutionGroup{
		ExecutionRole: "app_owner",
		Steps:         []plan.Step{{SQL: "CREATE INDEX CONCURRENTLY idx_documents_id ON public.documents (id)"}},
	}
	require.NoError(t, executeGroup(context.Background(), db, group, 1, true, lockRetryConfig{}))
	var valid bool
	require.NoError(t, db.QueryRow(`SELECT indisvalid FROM pg_index WHERE indexrelid = 'public.idx_documents_id'::regclass`).Scan(&valid))
	require.True(t, valid)
}
