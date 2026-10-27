package postgres

// Test-only hooks into unexported package state.
var (
	FindAvailablePort            = &findAvailablePort
	MapToEmbeddedPostgresVersion = mapToEmbeddedPostgresVersion
)
