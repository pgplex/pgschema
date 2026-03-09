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

// GetIRFromDatabase gets the IR from a database with ignore configuration
func GetIRFromDatabase(host string, port int, db, user, password, sslmode, schemaName, applicationName string, ignoreConfig *ir.IgnoreConfig) (*ir.IR, error) {
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

	// Build IR using the IR system with ignore config
	inspector := ir.NewInspector(conn, ignoreConfig)

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
