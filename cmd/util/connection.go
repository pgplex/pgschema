package util

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/internal/logger"
	"github.com/pgplex/pgschema/ir"
)

// ConnectionConfig holds database connection parameters
type ConnectionConfig struct {
	Host            string
	Port            int
	Database        string
	User            string
	Password        string
	SSLMode         string
	ApplicationName string
}

// Connect establishes a database connection using the provided configuration
func Connect(config *ConnectionConfig) (*sql.DB, error) {
	log := logger.Get()

	log.Debug("Attempting database connection",
		"host", config.Host,
		"port", config.Port,
		"database", config.Database,
		"user", config.User,
		"sslmode", config.SSLMode,
		"application_name", config.ApplicationName,
	)

	dsn := buildDSN(config)
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Debug("Database connection failed", "error", err)
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test the connection with a timeout to fail fast if the database is unreachable
	pingCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := conn.PingContext(pingCtx); err != nil {
		log.Debug("Database ping failed", "error", err)
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Debug("Database connection established successfully")
	return conn, nil
}

// buildDSN constructs a PostgreSQL connection string from connection parameters
func buildDSN(config *ConnectionConfig) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("host=%s", config.Host))
	parts = append(parts, fmt.Sprintf("port=%d", config.Port))
	parts = append(parts, fmt.Sprintf("dbname=%s", config.Database))
	parts = append(parts, fmt.Sprintf("user=%s", config.User))

	if config.Password != "" {
		parts = append(parts, fmt.Sprintf("password=%s", config.Password))
	}

	if config.SSLMode != "" {
		parts = append(parts, fmt.Sprintf("sslmode=%s", config.SSLMode))
	}

	if config.ApplicationName != "" {
		parts = append(parts, fmt.Sprintf("application_name=%s", config.ApplicationName))
	}

	// Set connect_timeout to fail fast if the database is unreachable.
	// This is a libpq parameter that pgx respects for TCP connection establishment.
	parts = append(parts, "connect_timeout=30")

	return strings.Join(parts, " ")
}

// ValidateSSLMode validates that the given sslmode is a valid PostgreSQL SSL mode.
func ValidateSSLMode(mode string) error {
	switch mode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return nil
	default:
		return fmt.Errorf("invalid sslmode %q: must be one of disable, allow, prefer, require, verify-ca, verify-full", mode)
	}
}

// GetIRFromDatabase gets the IR from a database with ignore configuration.
// managedSchema is the logical schema this IR represents when it differs
// from schemaName - the schema actually being connected to and introspected
// (see Inspector.SetManagedSchema). Pass "" when they're the same, which is
// every caller except the desired-state/temp-schema comparison path.
func GetIRFromDatabase(host string, port int, db, user, password, sslmode, schemaName, applicationName string, ignoreConfig *ir.IgnoreConfig, managedSchema string) (*ir.IR, error) {
	return GetIRFromDatabaseAsRole(
		host, port, db, user, password, sslmode, schemaName, applicationName,
		ignoreConfig, managedSchema, "",
	)
}

// GetIRFromDatabaseAsRole inspects through a SET-enabled owner role when it exists.
func GetIRFromDatabaseAsRole(host string, port int, db, user, password, sslmode, schemaName, applicationName string, ignoreConfig *ir.IgnoreConfig, managedSchema, inspectionRole string) (*ir.IR, error) {
	if sslmode == "" {
		sslmode = "prefer"
	}

	// Build database connection
	config := &ConnectionConfig{
		Host:            host,
		Port:            port,
		Database:        db,
		User:            user,
		Password:        password,
		SSLMode:         sslmode,
		ApplicationName: applicationName,
	}

	conn, err := Connect(config)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx := context.Background()
	if inspectionRole != "" {
		// SET ROLE is connection-local, while the inspector runs concurrent queries.
		conn.SetMaxOpenConns(1)
		conn.SetMaxIdleConns(1)
		var exists bool
		if err := conn.QueryRowContext(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", inspectionRole,
		).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check inspection role %q: %w", inspectionRole, err)
		}
		if exists {
			setPrivilege := "SET"
			var serverVersion int
			if err := conn.QueryRowContext(ctx, "SHOW server_version_num").Scan(&serverVersion); err != nil {
				return nil, fmt.Errorf("detect PostgreSQL version for inspection role: %w", err)
			}
			if serverVersion < 160000 {
				setPrivilege = "MEMBER"
			}
			var canSet bool
			if err := conn.QueryRowContext(ctx,
				"SELECT pg_has_role(current_user, $1, $2)", inspectionRole, setPrivilege,
			).Scan(&canSet); err != nil {
				return nil, fmt.Errorf("check SET authority for inspection role %q: %w", inspectionRole, err)
			}
			if !canSet {
				return nil, fmt.Errorf("session role cannot SET ROLE to schema owner %q for inspection", inspectionRole)
			}
			if _, err := conn.ExecContext(ctx, "SET ROLE "+ir.QuoteIdentifier(inspectionRole)); err != nil {
				return nil, fmt.Errorf("set inspection role %q: %w", inspectionRole, err)
			}
		}
	}

	// Build IR using the IR system with ignore config
	inspector := ir.NewInspector(conn, ignoreConfig)
	if managedSchema != "" {
		inspector.SetManagedSchema(managedSchema)
	}

	// Default to public schema if none specified
	targetSchema := schemaName
	if targetSchema == "" {
		targetSchema = "public"
	}

	schemaIR, err := inspector.BuildIR(ctx, targetSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to build IR: %w", err)
	}

	return schemaIR, nil
}
