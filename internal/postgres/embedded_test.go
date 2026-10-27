package postgres_test

import (
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

// TestStartEmbeddedPostgres_RetriesWhenPortIsTaken hands the first launch
// attempt a port that is already in use, as happens when another process grabs
// the probed port before pg_ctl binds it, and expects the next attempt to come
// up on a fresh port instead of failing the whole start.
func TestStartEmbeddedPostgres_RetriesWhenPortIsTaken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	held, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer held.Close()
	taken := held.Addr().(*net.TCPAddr).Port

	orig := *postgres.FindAvailablePort
	defer func() { *postgres.FindAvailablePort = orig }()
	calls := 0
	*postgres.FindAvailablePort = func() (int, error) {
		calls++
		if calls == 1 {
			return taken, nil
		}
		return orig()
	}

	major := testutil.LatestPostgresVersion
	if v := os.Getenv("PGSCHEMA_POSTGRES_VERSION"); v != "" {
		major, err = strconv.Atoi(v)
		require.NoError(t, err)
	}
	version, err := postgres.MapToEmbeddedPostgresVersion(major)
	require.NoError(t, err)

	embeddedPG, err := postgres.StartEmbeddedPostgres(&postgres.EmbeddedPostgresConfig{
		Version:  version,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
	})
	require.NoError(t, err, "second attempt should start on a fresh port")
	defer embeddedPG.Stop()

	_, port, _, _, _ := embeddedPG.GetConnectionDetails()
	require.Equal(t, 2, calls, "expected exactly one retry")
	require.NotEqual(t, taken, port)
}
