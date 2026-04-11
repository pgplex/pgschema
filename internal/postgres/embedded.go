// Package postgres provides embedded PostgreSQL functionality for production use.
// This package is used by the plan command to create temporary PostgreSQL instances
// for validating desired state schemas.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/cmd/util"
)

// PostgresVersion is an alias for the embedded-postgres version type.
type PostgresVersion = embeddedpostgres.PostgresVersion

// EmbeddedPostgres manages a temporary embedded PostgreSQL instance.
// This is used by the plan command to validate desired state schemas.
type EmbeddedPostgres struct {
	instance    *embeddedpostgres.EmbeddedPostgres
	db          *sql.DB
	version     PostgresVersion
	host        string
	port        int
	database    string
	username    string
	password    string
	runtimePath string
	tempSchema  string // temporary schema name with timestamp for uniqueness
}

// EmbeddedPostgresConfig holds configuration for starting embedded PostgreSQL
type EmbeddedPostgresConfig struct {
	Version  PostgresVersion
	Database string
	Username string
	Password string
}

// DetectPostgresVersionFromDB connects to a database and detects its version
// This is a convenience function that opens a connection, detects the version, and closes it
func DetectPostgresVersionFromDB(host string, port int, database, user, password, sslmode string) (PostgresVersion, error) {
	// Build connection config
	finalSSLMode := sslmode
	if finalSSLMode == "" {
		finalSSLMode = "prefer"
	}
	config := &util.ConnectionConfig{
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
		Password: password,
		SSLMode:  finalSSLMode,
	}

	// Connect to database
	db, err := util.Connect(config)
	if err != nil {
		return "", fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Detect version
	return detectPostgresVersion(db)
}

// StartEmbeddedPostgres starts a temporary embedded PostgreSQL instance
func StartEmbeddedPostgres(config *EmbeddedPostgresConfig) (*EmbeddedPostgres, error) {
	// Create unique runtime path and schema name
	tempSchema := GenerateTempSchemaName()
	runtimePath := filepath.Join(os.TempDir(), tempSchema)

	// Find an available port
	port, err := findAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to find available port: %w", err)
	}

	// Configure embedded postgres
	pgConfig := embeddedpostgres.DefaultConfig().
		Version(config.Version).
		Database(config.Database).
		Username(config.Username).
		Password(config.Password).
		Port(uint32(port)).
		RuntimePath(runtimePath).
		DataPath(filepath.Join(runtimePath, "data")).
		Logger(io.Discard). // Suppress embedded-postgres startup logs
		StartParameters(map[string]string{
			"logging_collector":          "off",    // Disable log collector
			"log_destination":            "stderr", // Send logs to stderr (which we discard)
			"log_min_messages":           "PANIC",  // Only log PANIC level messages
			"log_statement":              "none",   // Don't log SQL statements
			"log_min_duration_statement": "-1",     // Don't log slow queries
		})

	// Create and start PostgreSQL instance
	instance := embeddedpostgres.NewDatabase(pgConfig)
	if err := instance.Start(); err != nil {
		return nil, fmt.Errorf("failed to start embedded PostgreSQL: %w", err)
	}

	// Build connection config
	host := "localhost"
	connConfig := &util.ConnectionConfig{
		Host:     host,
		Port:     port,
		Database: config.Database,
		User:     config.Username,
		Password: config.Password,
		SSLMode:  "disable",
	}

	// Connect to database
	db, err := util.Connect(connConfig)
	if err != nil {
		instance.Stop()
		os.RemoveAll(runtimePath)
		return nil, fmt.Errorf("failed to connect to embedded PostgreSQL: %w", err)
	}

	return &EmbeddedPostgres{
		instance:    instance,
		db:          db,
		version:     config.Version,
		host:        host,
		port:        port,
		database:    config.Database,
		username:    config.Username,
		password:    config.Password,
		runtimePath: runtimePath,
		tempSchema:  tempSchema,
	}, nil
}

// Stop stops and cleans up the embedded PostgreSQL instance
func (ep *EmbeddedPostgres) Stop() error {
	// Drop the temporary schema (best effort - don't fail if this errors)
	if ep.db != nil && ep.tempSchema != "" {
		ctx := context.Background()
		dropSchemaSQL := fmt.Sprintf("DROP SCHEMA IF EXISTS \"%s\" CASCADE", ep.tempSchema)
		// Ignore errors - this is best effort cleanup
		_, _ = ep.db.ExecContext(ctx, dropSchemaSQL)
	}

	// Close database connection
	if ep.db != nil {
		ep.db.Close()
	}

	// Stop PostgreSQL instance
	var stopErr error
	if ep.instance != nil {
		stopErr = ep.instance.Stop()
	}

	// Clean up runtime directory
	if ep.runtimePath != "" {
		if err := os.RemoveAll(ep.runtimePath); err != nil {
			// Don't return error here - just ignore cleanup failures
			// This can happen on Windows when files are still in use
		}
	}

	if stopErr != nil {
		return fmt.Errorf("failed to stop embedded PostgreSQL: %w", stopErr)
	}

	return nil
}

// GetConnectionDetails returns all connection details needed to connect to the embedded PostgreSQL instance
func (ep *EmbeddedPostgres) GetConnectionDetails() (host string, port int, database, username, password string) {
	return ep.host, ep.port, ep.database, ep.username, ep.password
}

// GetSchemaName returns the temporary schema name used for desired state validation.
// This returns the timestamped schema name that was created by ApplySchema.
func (ep *EmbeddedPostgres) GetSchemaName() string {
	return ep.tempSchema
}

// ApplySchema resets a schema (drops and recreates it) and applies SQL to it.
// This ensures a clean state before applying the desired schema definition.
// Note: The schema parameter is ignored - we always use the temporary schema name.
func (ep *EmbeddedPostgres) ApplySchema(ctx context.Context, schema string, sql string) error {
	// Acquire a single dedicated connection to ensure SET search_path affects
	// all subsequent statements. Using *sql.DB (connection pool) does not
	// guarantee the same connection across ExecContext calls, so session-scoped
	// settings like search_path may be lost.
	conn, err := ep.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Close()

	// Drop the temporary schema if it exists (CASCADE to drop all objects)
	dropSchemaSQL := fmt.Sprintf("DROP SCHEMA IF EXISTS \"%s\" CASCADE", ep.tempSchema)
	if _, err := util.ExecContextWithLogging(ctx, conn, dropSchemaSQL, "drop temporary schema"); err != nil {
		return fmt.Errorf("failed to drop temporary schema %s: %w", ep.tempSchema, err)
	}

	// Create the temporary schema
	createSchemaSQL := fmt.Sprintf("CREATE SCHEMA \"%s\"", ep.tempSchema)
	if _, err := util.ExecContextWithLogging(ctx, conn, createSchemaSQL, "create temporary schema"); err != nil {
		return fmt.Errorf("failed to create temporary schema %s: %w", ep.tempSchema, err)
	}

	// Set search_path to the temporary schema, with public as fallback
	// for resolving extension types installed in public schema (issue #197)
	setSearchPathSQL := fmt.Sprintf("SET search_path TO \"%s\", public", ep.tempSchema)
	if _, err := util.ExecContextWithLogging(ctx, conn, setSearchPathSQL, "set search_path for desired state"); err != nil {
		return fmt.Errorf("failed to set search_path: %w", err)
	}

	// Disable function body validation to avoid type-identity mismatches (issue #399).
	// Schema qualifications inside dollar-quoted function bodies are preserved (issue #354),
	// but parameter types are stripped. For SQL-language functions, PostgreSQL validates the
	// body at creation time, which can fail when body references use the original schema's
	// types while parameters reference the temporary schema's types.
	if _, err := util.ExecContextWithLogging(ctx, conn, "SET check_function_bodies = off", "disable function body validation for desired state"); err != nil {
		return fmt.Errorf("failed to disable check_function_bodies: %w", err)
	}

	// Strip schema qualifications from SQL before applying to temporary schema
	// This ensures that objects are created in the temporary schema via search_path
	// rather than being explicitly qualified with the original schema name
	schemaAgnosticSQL := stripSchemaQualifications(sql, schema)

	// Replace schema names in ALTER DEFAULT PRIVILEGES statements
	// These use "IN SCHEMA <schema>" syntax which isn't handled by stripSchemaQualifications
	schemaAgnosticSQL = replaceSchemaInDefaultPrivileges(schemaAgnosticSQL, schema, ep.tempSchema)

	// Replace schema names in SET search_path clauses within function/procedure definitions
	// SQL-language functions are validated at creation time using the function's own search_path,
	// so we need to rewrite it to point to the temporary schema (issue #335)
	schemaAgnosticSQL = replaceSchemaInSearchPath(schemaAgnosticSQL, schema, ep.tempSchema)

	// Execute the SQL directly
	// Note: Desired state SQL should never contain operations like CREATE INDEX CONCURRENTLY
	// that cannot run in transactions. Those are migration details, not state declarations.
	if _, err := util.ExecContextWithLogging(ctx, conn, schemaAgnosticSQL, "apply desired state SQL to temporary schema"); err != nil {
		return fmt.Errorf("failed to apply schema SQL to temporary schema %s: %w", ep.tempSchema, enhanceApplyError(err, schemaAgnosticSQL))
	}

	return nil
}

// findAvailablePort finds an available TCP port for PostgreSQL to use
func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// mapToEmbeddedPostgresVersion maps a PostgreSQL major version to embedded-postgres version
// Supported versions: 14, 15, 16, 17, 18
func mapToEmbeddedPostgresVersion(majorVersion int) (PostgresVersion, error) {
	switch majorVersion {
	case 14:
		return embeddedpostgres.V14, nil
	case 15:
		return embeddedpostgres.V15, nil
	case 16:
		return embeddedpostgres.V16, nil
	case 17:
		return embeddedpostgres.V17, nil
	case 18:
		return embeddedpostgres.V18, nil
	default:
		return "", fmt.Errorf("unsupported PostgreSQL version %d (supported: 14-18)", majorVersion)
	}
}

// detectPostgresVersion queries the target database to determine its PostgreSQL version
// and returns the corresponding embedded-postgres version string
func detectPostgresVersion(db *sql.DB) (PostgresVersion, error) {
	ctx := context.Background()

	// Query PostgreSQL version number (e.g., 170005 for 17.5)
	var versionNum int
	err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&versionNum)
	if err != nil {
		return "", fmt.Errorf("failed to query PostgreSQL version: %w", err)
	}

	// Extract major version: version_num / 10000
	// e.g., 170005 / 10000 = 17
	majorVersion := versionNum / 10000

	// Map to embedded-postgres version
	return mapToEmbeddedPostgresVersion(majorVersion)
}
