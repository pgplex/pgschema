package postgres_test

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

func testEmbeddedConfig(t *testing.T) *postgres.EmbeddedPostgresConfig {
	t.Helper()
	major := testutil.LatestPostgresVersion
	if v := os.Getenv("PGSCHEMA_POSTGRES_VERSION"); v != "" {
		var err error
		major, err = strconv.Atoi(v)
		require.NoError(t, err)
	}
	version, err := postgres.MapToEmbeddedPostgresVersion(major)
	require.NoError(t, err)
	return &postgres.EmbeddedPostgresConfig{
		Version:  version,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
	}
}

// holdPort returns a port that stays occupied for the rest of the test.
func holdPort(t *testing.T) int {
	t.Helper()
	held, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	t.Cleanup(func() { held.Close() })
	return held.Addr().(*net.TCPAddr).Port
}

// stubFindAvailablePort replaces the port picker for the test and returns a
// counter of how many times it was called.
func stubFindAvailablePort(t *testing.T, pick func(call int, orig func() (int, error)) (int, error)) *int {
	t.Helper()
	orig := *postgres.FindAvailablePort
	t.Cleanup(func() { *postgres.FindAvailablePort = orig })
	calls := 0
	*postgres.FindAvailablePort = func() (int, error) {
		calls++
		return pick(calls, orig)
	}
	return &calls
}

// TestStartEmbeddedPostgres_RetriesWhenPortIsTaken hands the first launch
// attempt a port that is already in use, as happens when another process grabs
// the probed port before pg_ctl binds it, and expects the next attempt to come
// up on a fresh port instead of failing the whole start.
func TestStartEmbeddedPostgres_RetriesWhenPortIsTaken(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	taken := holdPort(t)
	calls := stubFindAvailablePort(t, func(call int, orig func() (int, error)) (int, error) {
		if call == 1 {
			return taken, nil
		}
		return orig()
	})

	embeddedPG, err := postgres.StartEmbeddedPostgres(testEmbeddedConfig(t))
	require.NoError(t, err, "second attempt should start on a fresh port")
	defer embeddedPG.Stop()

	_, port, _, _, _ := embeddedPG.GetConnectionDetails()
	require.Equal(t, 2, *calls, "expected exactly one retry")
	require.NotEqual(t, taken, port)
}

// TestStartEmbeddedPostgres_GivesUpAfterRetries keeps every attempt on the
// taken port and expects the start to fail after the bounded number of
// attempts rather than retry forever.
func TestStartEmbeddedPostgres_GivesUpAfterRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	taken := holdPort(t)
	calls := stubFindAvailablePort(t, func(int, func() (int, error)) (int, error) {
		return taken, nil
	})

	embeddedPG, err := postgres.StartEmbeddedPostgres(testEmbeddedConfig(t))
	require.Error(t, err)
	require.Nil(t, embeddedPG)
	require.Equal(t, 3, *calls)
}

// TestStopOrphanedPostmaster verifies the fallback stop finds pg_ctl under the
// runtime directory and brings a running server down, so a failed launch that
// left a postmaster behind does not get its data directory deleted from under
// it.
func TestStopOrphanedPostmaster(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	embeddedPG, err := postgres.StartEmbeddedPostgres(testEmbeddedConfig(t))
	require.NoError(t, err)
	defer embeddedPG.Stop() // fails because the server is already down, but still removes the directory

	runtimePath := embeddedPG.RuntimePath()
	_, port, _, _, _ := embeddedPG.GetConnectionDetails()
	_, err = os.Stat(filepath.Join(runtimePath, "data", "postmaster.pid"))
	require.NoError(t, err, "server should be running before the stop")

	postgres.StopOrphanedPostmaster(runtimePath)

	_, err = os.Stat(filepath.Join(runtimePath, "data", "postmaster.pid"))
	require.True(t, os.IsNotExist(err), "postmaster.pid should be gone after the stop")
	_, err = net.Dial("tcp", fmt.Sprintf("localhost:%d", port))
	require.Error(t, err, "nothing should be listening on the server's port")
}
