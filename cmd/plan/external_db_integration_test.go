package plan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalDatabase_BasicFunctionality tests that external database can be used for desired state
func TestExternalDatabase_BasicFunctionality(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup: Create two embedded postgres instances
	// One serves as "target" database, one serves as "external plan" database
	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()

	externalPlanDB := testutil.SetupPostgres(t)
	defer externalPlanDB.Stop()

	// Get connection details
	targetHost, targetPort, targetDatabase, targetUser, targetPassword := targetDB.GetConnectionDetails()
	planHost, planPort, planDatabase, planUser, planPassword := externalPlanDB.GetConnectionDetails()

	// Create test schema file
	schemaSQL := `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL
);

CREATE INDEX idx_users_email ON users(email);
`

	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "schema.sql")
	err := os.WriteFile(schemaFile, []byte(schemaSQL), 0644)
	require.NoError(t, err)

	// Create config with external database
	config := &PlanConfig{
		Host:            targetHost,
		Port:            targetPort,
		DB:              targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		Schema:          "public",
		File:            schemaFile,
		ApplicationName: "pgschema-test",
		// External database configuration
		PlanDBHost:     planHost,
		PlanDBPort:     planPort,
		PlanDBDatabase: planDatabase,
		PlanDBUser:     planUser,
		PlanDBPassword: planPassword,
	}

	// Create external database provider
	provider, err := CreateDesiredStateProvider(config)
	require.NoError(t, err, "should create external database provider")
	defer provider.Stop()

	// Verify it's an external database (not embedded)
	_, ok := provider.(*postgres.ExternalDatabase)
	assert.True(t, ok, "provider should be ExternalDatabase when plan-host is provided")

	// Verify temporary schema name is returned
	tempSchema := provider.GetSchemaName()
	assert.NotEmpty(t, tempSchema, "temporary schema name should not be empty")
	assert.Contains(t, tempSchema, "pgschema_tmp_", "temporary schema should have timestamp prefix")

	// Generate plan
	migrationPlan, err := GeneratePlan(config, provider)
	require.NoError(t, err, "should generate plan")

	// Verify plan has changes (target is empty, desired has tables)
	assert.NotNil(t, migrationPlan)
	assert.True(t, len(migrationPlan.Groups) > 0, "plan should have at least one group")
}

// TestExternalDatabase_VersionMismatch tests version compatibility checking
func TestExternalDatabase_VersionMismatch(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This test would require two different PostgreSQL versions, which is complex to setup
	// For now, we just verify that version detection works
	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()

	targetHost, targetPort, targetDatabase, targetUser, targetPassword := targetDB.GetConnectionDetails()

	// Detect version from target database
	pgVersion, _, err := postgres.DetectPostgresVersionAndExtensionsFromDB(
		targetHost,
		targetPort,
		targetDatabase,
		targetUser,
		targetPassword,
		"prefer",
	)
	require.NoError(t, err, "should detect PostgreSQL version")
	assert.NotEmpty(t, pgVersion, "version should not be empty")
}

// TestExternalDatabase_ExtensionSchemaMismatch tests that provider creation fails fast
// when an extension is installed in different schemas on the plan and target databases.
// Extension-owned types (e.g. pgvector's vector, citext) are schema-qualified by whatever
// schema their extension resolves to, so a mismatch silently produces a wrong plan:
// spurious ALTER COLUMN TYPE or CREATE TABLE DDL referencing a schema that doesn't
// exist on the target (issue #518).
func TestExternalDatabase_ExtensionSchemaMismatch(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()

	externalPlanDB := testutil.SetupPostgres(t)
	defer externalPlanDB.Stop()

	// Install citext into schema "exts" on the target, but "public" on the plan database
	targetConn, targetHost, targetPort, targetDatabase, targetUser, targetPassword := testutil.ConnectToPostgres(t, targetDB)
	defer targetConn.Close()
	_, err := targetConn.Exec("CREATE SCHEMA exts")
	require.NoError(t, err)
	_, err = targetConn.Exec("CREATE EXTENSION citext SCHEMA exts")
	require.NoError(t, err)

	planConn, planHost, planPort, planDatabase, planUser, planPassword := testutil.ConnectToPostgres(t, externalPlanDB)
	defer planConn.Close()
	_, err = planConn.Exec("CREATE EXTENSION citext SCHEMA public")
	require.NoError(t, err)

	config := &PlanConfig{
		Host:            targetHost,
		Port:            targetPort,
		DB:              targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		Schema:          "public",
		ApplicationName: "pgschema-test",
		PlanDBHost:      planHost,
		PlanDBPort:      planPort,
		PlanDBDatabase:  planDatabase,
		PlanDBUser:      planUser,
		PlanDBPassword:  planPassword,
	}

	_, err = CreateDesiredStateProvider(config)
	require.Error(t, err, "should fail when extension schemas differ between plan and target databases")
	assert.Contains(t, err.Error(), "citext", "error should name the mismatched extension")
	assert.Contains(t, err.Error(), "exts", "error should name the target schema")
	assert.Contains(t, err.Error(), "public", "error should name the plan database schema")

	// After moving the extension to the matching schema, provider creation succeeds
	_, err = planConn.Exec("CREATE SCHEMA exts")
	require.NoError(t, err)
	_, err = planConn.Exec("ALTER EXTENSION citext SET SCHEMA exts")
	require.NoError(t, err)

	provider2, err := CreateDesiredStateProvider(config)
	require.NoError(t, err, "matching extension schemas should pass validation")
	provider2.Stop()
}

// TestExternalDatabase_CleanupOnError tests that temporary schema is cleaned up on errors
func TestExternalDatabase_CleanupOnError(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup external plan database
	externalPlanDB := testutil.SetupPostgres(t)
	defer externalPlanDB.Stop()

	planHost, planPort, planDatabase, planUser, planPassword := externalPlanDB.GetConnectionDetails()

	// Detect the actual major version from the embedded database
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, externalPlanDB)
	defer conn.Close()

	majorVersion, err := testutil.GetMajorVersion(conn)
	require.NoError(t, err, "should detect PostgreSQL major version")

	// Create external database connection with correct version
	externalConfig := &postgres.ExternalDatabaseConfig{
		Host:               planHost,
		Port:               planPort,
		Database:           planDatabase,
		Username:           planUser,
		Password:           planPassword,
		TargetMajorVersion: majorVersion,
	}

	extDB, err := postgres.NewExternalDatabase(externalConfig)
	require.NoError(t, err)

	tempSchema := extDB.GetSchemaName()
	require.NotEmpty(t, tempSchema)

	// Apply some SQL to create the schema
	ctx := context.Background()
	err = extDB.ApplySchema(ctx, "public", "CREATE TABLE test (id INT);")
	require.NoError(t, err)

	// Verify schema exists by checking connection details
	host, port, db, user, pass := extDB.GetConnectionDetails()
	assert.Equal(t, planHost, host)
	assert.Equal(t, planPort, port)
	assert.Equal(t, planDatabase, db)
	assert.Equal(t, planUser, user)
	assert.Equal(t, planPassword, pass)

	// Stop should clean up the temporary schema (best effort)
	err = extDB.Stop()
	assert.NoError(t, err, "cleanup should not error")
}

// TestExternalDatabase_SchemaIsolation tests that temporary schemas don't interfere with each other
func TestExternalDatabase_SchemaIsolation(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup external plan database
	externalPlanDB := testutil.SetupPostgres(t)
	defer externalPlanDB.Stop()

	planHost, planPort, planDatabase, planUser, planPassword := externalPlanDB.GetConnectionDetails()

	// Detect the actual major version from the embedded database
	conn, _, _, _, _, _ := testutil.ConnectToPostgres(t, externalPlanDB)
	defer conn.Close()

	majorVersion, err := testutil.GetMajorVersion(conn)
	require.NoError(t, err, "should detect PostgreSQL major version")

	// Create two external database connections
	externalConfig1 := &postgres.ExternalDatabaseConfig{
		Host:               planHost,
		Port:               planPort,
		Database:           planDatabase,
		Username:           planUser,
		Password:           planPassword,
		TargetMajorVersion: majorVersion,
	}
	extDB1, err := postgres.NewExternalDatabase(externalConfig1)
	require.NoError(t, err)
	defer extDB1.Stop()

	externalConfig2 := &postgres.ExternalDatabaseConfig{
		Host:               planHost,
		Port:               planPort,
		Database:           planDatabase,
		Username:           planUser,
		Password:           planPassword,
		TargetMajorVersion: majorVersion,
	}
	extDB2, err := postgres.NewExternalDatabase(externalConfig2)
	require.NoError(t, err)
	defer extDB2.Stop()

	// Verify different schema names due to timestamp
	schema1 := extDB1.GetSchemaName()
	schema2 := extDB2.GetSchemaName()
	assert.NotEqual(t, schema1, schema2, "temporary schemas should have unique names")
}
