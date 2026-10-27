package postgres

// Test-only hooks into unexported package state.
var (
	FindAvailablePort            = &findAvailablePort
	MapToEmbeddedPostgresVersion = mapToEmbeddedPostgresVersion
	StopOrphanedPostmaster       = stopOrphanedPostmaster
)

// RuntimePath exposes the instance's runtime directory to tests.
func (ep *EmbeddedPostgres) RuntimePath() string { return ep.runtimePath }
