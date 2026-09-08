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
	"sort"
	"strings"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/logger"
)

// binariesPath is the path that contains the Postgres binaries.
// This is meant to be injected via ldflags (for example, to use the nix
// postgres versions when building with nix).
var binariesPath string

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
	tempSchema  string            // temporary schema name with timestamp for uniqueness
	extensions  map[string]string // target extensions to mirror, name -> schema (issue #584)
}

// EmbeddedPostgresConfig holds configuration for starting embedded PostgreSQL
type EmbeddedPostgresConfig struct {
	Version  PostgresVersion
	Database string
	Username string
	Password string
	// Extensions maps extension name to its installation schema on the target
	// database. ApplySchema installs each one into the embedded database before
	// applying the desired state, so desired-state SQL can reference extension
	// types without a CREATE EXTENSION statement (issue #584). Optional.
	Extensions map[string]string
}

// DetectPostgresVersionAndExtensionsFromDB connects to a database and detects its
// version along with the installation schema of every installed extension (keyed by
// extension name). Gathering both over a single connection avoids a second
// connect/close round trip during plan generation.
func DetectPostgresVersionAndExtensionsFromDB(host string, port int, database, user, password, sslmode string) (PostgresVersion, map[string]string, error) {
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
		return "", nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Detect version
	version, err := detectPostgresVersion(db)
	if err != nil {
		return "", nil, err
	}

	extensions, err := getExtensionSchemas(db)
	if err != nil {
		return "", nil, fmt.Errorf("failed to query extensions: %w", err)
	}

	return version, extensions, nil
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
		BinariesPath(binariesPath).
		DataPath(filepath.Join(runtimePath, "data")).
		Logger(io.Discard). // Suppress embedded-postgres startup logs
		StartParameters(map[string]string{
			"logging_collector":          "off",       // Disable log collector
			"log_destination":            "stderr",    // Send logs to stderr (which we discard)
			"log_min_messages":           "PANIC",     // Only log PANIC level messages
			"log_statement":              "none",      // Don't log SQL statements
			"log_min_duration_statement": "-1",        // Don't log slow queries
			"unix_socket_directories":    runtimePath, // Use a directory that is guaranteed to exist
			"timezone":                   "UTC",       // Ensure platform-independent output for pg_get_expr
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
		extensions:  config.Extensions,
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

	// Mirror the target's extensions so extension types resolve (issue #584)
	if err := ep.installTargetExtensions(ctx, conn, schema); err != nil {
		return err
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
	if err := ExecuteSchemaSQL(ctx, conn, schemaAgnosticSQL, schema); err != nil {
		enhanced := hintExtensionDependency(err, "this schema may depend on a PostgreSQL extension that the embedded plan database cannot provide. Extensions installed on the target database are mirrored automatically, but only those bundled with PostgreSQL (contrib) are available; for third-party extensions such as postgis or pgvector, use an external plan database with the extension installed (--plan-host), see https://www.pgschema.com/cli/plan-db")
		enhanced = hintCrossSchemaReference(enhanced, "this schema may reference objects in another schema that the embedded plan database does not have. If the table exists on the target database, add it to .pgschemaignore ([schemas] or schema-qualified [tables] pattern, e.g. auth or auth.users), see https://www.pgschema.com/cli/ignore. Otherwise add a stub CREATE SCHEMA/TABLE in your desired SQL, or use an external plan database (--plan-host), see https://www.pgschema.com/cli/plan-db")
		return fmt.Errorf("failed to apply schema SQL to temporary schema %s: %w", ep.tempSchema, enhanced)
	}

	return nil
}

// installTargetExtensions mirrors the target database's extensions into the
// embedded plan database. pgschema does not manage extensions (they are
// database-level objects), so a dump never emits CREATE EXTENSION; without this
// step, desired-state SQL that references an extension type fails with
// "type does not exist" (issue #584).
//
// Each extension is pinned to the schema it occupies on the target so that type
// qualification matches on both sides of the diff (issue #518). An extension
// living in the managed schema is installed into the temporary schema, which
// stands in for the managed schema during plan. Extensions the embedded binary
// does not bundle (e.g. postgis, pgvector) are skipped with a warning; if the
// desired state actually needs one, the later apply error points at --plan-host.
func (ep *EmbeddedPostgres) installTargetExtensions(ctx context.Context, conn *sql.Conn, managedSchema string) error {
	// Resolve each extension's schema in the plan database and make sure it exists.
	schemas := make(map[string]string, len(ep.extensions))
	pending := make([]string, 0, len(ep.extensions))
	for name, schema := range ep.extensions {
		switch {
		case name == "plpgsql":
			// Preinstalled in every database; nothing to mirror.
			continue
		case schema == managedSchema:
			schema = ep.tempSchema
		case strings.HasPrefix(schema, "pg_"):
			// System schemas (pg_catalog etc.) always exist and cannot be
			// created — the pg_ prefix is reserved, even with IF NOT EXISTS.
			// Extensions installed there (adminpack, or a relocatable one the
			// user pinned to pg_catalog) still need mirroring.
		default:
			createSchemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", quoteIdent(schema))
			if _, err := util.ExecContextWithLogging(ctx, conn, createSchemaSQL, "create schema for target extension"); err != nil {
				return fmt.Errorf("failed to create schema %s for extension %s: %w", schema, name, err)
			}
		}
		schemas[name] = schema
		pending = append(pending, name)
	}
	sort.Strings(pending)

	unavailable := installUntilFixpoint(pending, func(name string) error {
		createExtSQL := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s WITH SCHEMA %s", quoteIdent(name), quoteIdent(schemas[name]))
		_, err := util.ExecContextWithLogging(ctx, conn, createExtSQL, "install target extension")
		return err
	})
	for _, name := range pending {
		if err, ok := unavailable[name]; ok {
			logger.Get().Warn("extension installed on the target database is not available in the embedded plan database; if the schema depends on it, use an external plan database (--plan-host)",
				"extension", name, "error", err)
		}
	}
	return nil
}

// installUntilFixpoint calls install for every name, then retries the failures
// after each pass until a pass installs nothing more. It returns the last error
// for each name that never succeeded.
//
// Extensions can require others (hstore_plperl needs hstore and plperl), and the
// required one may sort later, so a single ordered pass is not enough. Rather
// than reconstructing the dependency graph from the target, keep retrying: each
// pass installs at least the prerequisites whose own prerequisites are met, so
// the loop terminates within len(names) passes. CASCADE is not an option — it
// would install prerequisites into the wrong schema.
func installUntilFixpoint(names []string, install func(name string) error) map[string]error {
	lastErr := make(map[string]error)
	pending := names
	for len(pending) > 0 {
		var failed []string
		for _, name := range pending {
			if err := install(name); err != nil {
				failed = append(failed, name)
				lastErr[name] = err
			} else {
				delete(lastErr, name)
			}
		}
		if len(failed) == len(pending) {
			break
		}
		pending = failed
	}
	return lastErr
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
