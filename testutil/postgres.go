// Package testutil provides shared test utilities for pgschema
package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// SetupPostgres creates a PostgreSQL instance for testing.
// It uses the production postgres.EmbeddedPostgres implementation.
// PostgreSQL version is determined from PGSCHEMA_POSTGRES_VERSION environment variable.
func SetupPostgres(t testing.TB) *postgres.EmbeddedPostgres {

	// Determine PostgreSQL version from environment
	version := getPostgresVersion()

	// Create configuration for production postgres package
	config := &postgres.EmbeddedPostgresConfig{
		Version:  version,
		Database: "testdb",
		Username: "testuser",
		Password: "testpass",
	}

	// Start embedded PostgreSQL using production code
	embeddedPG, err := postgres.StartEmbeddedPostgres(config)
	if err != nil {
		if t != nil {
			t.Fatalf("Failed to start embedded PostgreSQL: %v", err)
		} else {
			panic("Failed to start embedded PostgreSQL: " + err.Error())
		}
	}

	return embeddedPG
}

// ParseSQLToIR is a test helper that parses SQL and returns its IR representation.
// It applies the SQL to an embedded PostgreSQL instance, inspects it, and returns the IR.
// The schema will be reset (dropped and recreated) to ensure clean state between test calls.
// This ensures tests use the same code path as production (database inspection) rather than parsing.
func ParseSQLToIR(t *testing.T, embeddedPG *postgres.EmbeddedPostgres, sqlContent string, schema string) *ir.IR {
	return ParseSQLToIRWithSetup(t, embeddedPG, sqlContent, schema, "")
}

// ParseSQLToIRWithSetup is like ParseSQLToIR but accepts optional setup SQL.
// The setupSQL is executed AFTER schema recreation but BEFORE the main SQL.
// This is useful for tests that need to install extensions in the target schema
// (e.g., citext in public schema) which would otherwise be dropped during schema reset.
func ParseSQLToIRWithSetup(t *testing.T, embeddedPG *postgres.EmbeddedPostgres, sqlContent string, schema string, setupSQL string) *ir.IR {
	t.Helper()

	ctx := context.Background()

	// Get connection details from embedded postgres
	host, port, database, username, password := embeddedPG.GetConnectionDetails()

	// Build connection string
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		username, password, host, port, database)

	// Connect to database
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close()

	// Test the connection
	if err := conn.PingContext(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Drop and recreate schema for clean state
	dropSchema := fmt.Sprintf("DROP SCHEMA IF EXISTS \"%s\" CASCADE", schema)
	if _, err := conn.ExecContext(ctx, dropSchema); err != nil {
		t.Fatalf("Failed to drop schema: %v", err)
	}
	createSchema := fmt.Sprintf("CREATE SCHEMA \"%s\"", schema)
	if _, err := conn.ExecContext(ctx, createSchema); err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	// Set search_path to target schema, with public as fallback
	// for resolving extension types installed in public schema (issue #197)
	setSearchPathSQL := fmt.Sprintf("SET search_path TO \"%s\", public", schema)
	if _, err := conn.ExecContext(ctx, setSearchPathSQL); err != nil {
		t.Fatalf("Failed to set search_path: %v", err)
	}

	// Execute optional setup SQL (e.g., CREATE EXTENSION for types in public schema)
	// This runs after schema recreation so extensions won't be dropped
	if setupSQL != "" {
		if _, err := conn.ExecContext(ctx, setupSQL); err != nil {
			t.Fatalf("Failed to apply setup SQL to embedded PostgreSQL: %v", err)
		}
	}

	// Execute the SQL
	if _, err := conn.ExecContext(ctx, sqlContent); err != nil {
		t.Fatalf("Failed to apply SQL to embedded PostgreSQL: %v", err)
	}

	// Inspect the database to get IR
	inspector := ir.NewInspector(conn, nil)
	irResult, err := inspector.BuildIR(ctx, schema)
	if err != nil {
		t.Fatalf("Failed to inspect embedded PostgreSQL: %v", err)
	}

	return irResult
}

// ConnectToPostgres connects to an embedded PostgreSQL instance and returns connection details.
// This is a helper for tests that need database connection information.
// The caller is responsible for closing the returned *sql.DB connection.
func ConnectToPostgres(t testing.TB, embeddedPG *postgres.EmbeddedPostgres) (conn *sql.DB, host string, port int, dbname, user, password string) {
	t.Helper()

	ctx := context.Background()

	// Get connection details from embedded postgres
	host, port, dbname, user, password = embeddedPG.GetConnectionDetails()

	// Build connection string
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		user, password, host, port, dbname)

	// Connect to database
	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// Test the connection
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		t.Fatalf("Failed to ping database: %v", err)
	}

	return conn, host, port, dbname, user, password
}

// getPostgresVersion returns the PostgreSQL version to use for testing.
// It reads from the PGSCHEMA_POSTGRES_VERSION environment variable, defaulting
// to LatestPostgresVersion if not set (so the default run exercises the full suite).
func getPostgresVersion() postgres.PostgresVersion {
	versionStr := os.Getenv("PGSCHEMA_POSTGRES_VERSION")
	if versionStr == "" {
		versionStr = strconv.Itoa(LatestPostgresVersion)
	}
	switch versionStr {
	case "14":
		return embeddedpostgres.V14
	case "15":
		return embeddedpostgres.V15
	case "16":
		return embeddedpostgres.V16
	case "17":
		return embeddedpostgres.V17
	case "18":
		return embeddedpostgres.V18
	default:
		return embeddedpostgres.V18
	}
}

// GetMajorVersion detects the major version of a PostgreSQL database connection.
// It queries the database using SHOW server_version_num and extracts the major version.
// For example, version 170005 (17.5) returns 17.
func GetMajorVersion(db *sql.DB) (int, error) {
	ctx := context.Background()

	// Query PostgreSQL version number (e.g., 170005 for 17.5)
	var versionNum int
	err := db.QueryRowContext(ctx, "SHOW server_version_num").Scan(&versionNum)
	if err != nil {
		return 0, fmt.Errorf("failed to query PostgreSQL version: %w", err)
	}

	// Extract major version: version_num / 10000
	// e.g., 170005 / 10000 = 17
	majorVersion := versionNum / 10000

	return majorVersion, nil
}
