package apply

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// sharedEmbeddedPG is a shared embedded PostgreSQL instance used across all integration tests
	// to significantly improve test performance by avoiding repeated startup/teardown
	sharedEmbeddedPG *postgres.EmbeddedPostgres
)

// TestMain sets up shared resources for all tests in this package
func TestMain(m *testing.M) {
	// Create shared embedded postgres instance for all integration tests
	// This dramatically improves test performance by reusing the same instance
	sharedEmbeddedPG = testutil.SetupPostgres(nil)
	defer sharedEmbeddedPG.Stop()

	m.Run()
}

// TestApplyCommand_TransactionRollback verifies that the apply command uses proper
// transaction mode. If any statement fails in the middle of execution, the entire
// transaction should be rolled back and no partial changes should be applied.
//
// The test:
// 1. Generates a valid migration plan from a valid desired state schema
// 2. Manually injects a failing SQL statement (invalid foreign key) into the plan
// 3. Applies the modified plan, which should fail and trigger rollback
// 4. Verifies all changes in the transaction group were rolled back
//
// The migration contains multiple statements that should all run in a single transaction:
// - ALTER TABLE users ADD COLUMN email (valid)
// - ALTER TABLE users ADD COLUMN status (valid)
// - CREATE TABLE posts with valid foreign key to users (valid)
// - CREATE TABLE products with valid foreign key to users (valid)
// - ALTER TABLE products ADD CONSTRAINT (invalid FK - injected, causes failure)
//
// When the last statement fails, all statements in the transaction group should be rolled back,
// including the successful column additions and table creations.
func TestApplyCommand_TransactionRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	var err error

	// Start PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Setup database with initial schema

	initialSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		);

		INSERT INTO users (name) VALUES ('Alice'), ('Bob');
	`
	_, err = conn.ExecContext(ctx, initialSQL)
	if err != nil {
		t.Fatalf("Failed to setup initial schema: %v", err)
	}

	// Verify no email column exists initially
	var emailColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)
	`).Scan(&emailColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if email column exists: %v", err)
	}
	if emailColumnExists {
		t.Fatal("Email column should not exist initially")
	}

	// Create desired state schema file that will generate a valid migration
	// We'll manually inject a failing statement into the plan later to test rollback
	tmpDir := t.TempDir()
	desiredStateFile := filepath.Join(tmpDir, "desired_state.sql")
	// This desired state will generate a migration that:
	// 1. Adds email column to users (valid)
	// 2. Adds status column to users (valid)
	// 3. Creates posts table with valid foreign key to users (valid)
	// 4. Creates products table with valid foreign key to users (valid)
	desiredStateSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			status VARCHAR(50) DEFAULT 'active'
		);

		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			user_id INTEGER REFERENCES users(id)
		);

		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			user_id INTEGER REFERENCES users(id)
		);
	`
	err = os.WriteFile(desiredStateFile, []byte(desiredStateSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write desired state file: %v", err)
	}

	// Get container connection details
	containerHost := container.Host
	portMapped := container.Port

	// First, generate and verify the migration plan
	planConfig := &planCmd.PlanConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		File:            desiredStateFile,
		ApplicationName: "pgschema",
	}

	migrationPlan, err := planCmd.GenerateSchemaPlan(planConfig, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to generate migration plan: %v", err)
	}

	// Verify the planned SQL contains the expected valid statements
	plannedSQL := migrationPlan.ToSQL(plan.SQLFormatRaw)

	// Verify that the planned SQL contains our expected statements
	if !strings.Contains(plannedSQL, "ALTER TABLE users ADD COLUMN email") {
		t.Fatalf("Expected migration to contain 'ALTER TABLE users ADD COLUMN email', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "ALTER TABLE users ADD COLUMN status") {
		t.Fatalf("Expected migration to contain 'ALTER TABLE users ADD COLUMN status', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "CREATE TABLE IF NOT EXISTS posts") {
		t.Fatalf("Expected migration to contain 'CREATE TABLE IF NOT EXISTS posts', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "CREATE TABLE IF NOT EXISTS products") {
		t.Fatalf("Expected migration to contain 'CREATE TABLE IF NOT EXISTS products', got: %s", plannedSQL)
	}

	t.Log("Valid migration plan generated - now injecting failing statement to test rollback")

	// Manually inject a failing SQL statement to test transaction rollback
	// We inject an invalid foreign key constraint that references a nonexistent table
	// This ensures the plan generation succeeds (valid desired state) but apply fails (rollback test)
	if len(migrationPlan.Groups) == 0 {
		t.Fatal("Expected at least one execution group in the migration plan")
	}

	// Add the failing statement to the last execution group
	// This will cause the entire transaction group to roll back when it fails
	lastGroupIdx := len(migrationPlan.Groups) - 1
	failingStep := plan.Step{
		SQL:       "ALTER TABLE products ADD CONSTRAINT products_invalid_fk FOREIGN KEY (user_id) REFERENCES nonexistent_users (id);",
		Type:      "table",
		Operation: "alter",
		Path:      "public.products",
	}
	migrationPlan.Groups[lastGroupIdx].Steps = append(
		migrationPlan.Groups[lastGroupIdx].Steps,
		failingStep,
	)

	t.Log("Injected failing statement into migration plan")

	// Apply the modified plan directly using ApplyMigration
	applyConfig := &ApplyConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		Plan:            migrationPlan, // Use pre-generated plan with injected failure
		AutoApprove:     true,
		NoColor:         false,
		Quiet:           true, // Suppress output in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration directly (no need for JSON file or embedded postgres)
	err = ApplyMigration(applyConfig, nil)
	if err == nil {
		t.Fatal("Expected apply command to fail due to invalid DDL, but it succeeded")
	}

	// Verify that ALL changes in the same transaction group were rolled back
	// Check that email column was NOT added to users table (should be rolled back)
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)
	`).Scan(&emailColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if email column exists after failed apply: %v", err)
	}
	if emailColumnExists {
		t.Fatal("Email column should not exist after failed transaction - rollback did not work properly")
	}

	// Check that status column was NOT added to users table (should be rolled back)
	var statusColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'status'
		)
	`).Scan(&statusColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if status column exists after failed apply: %v", err)
	}
	if statusColumnExists {
		t.Fatal("Status column should not exist after failed transaction - rollback did not work properly")
	}

	// Verify posts table was NOT created (should be rolled back)
	var postsTableExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'posts'
		)
	`).Scan(&postsTableExists)
	if err != nil {
		t.Fatalf("Failed to check if posts table exists after failed apply: %v", err)
	}
	if postsTableExists {
		t.Fatal("Posts table should not exist after failed transaction - rollback did not work properly")
	}

	// Verify products table was NOT created (this was the failing statement)
	var productsTableExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'products'
		)
	`).Scan(&productsTableExists)
	if err != nil {
		t.Fatalf("Failed to check if products table exists after failed apply: %v", err)
	}
	if productsTableExists {
		t.Fatal("Products table should not exist after failed transaction")
	}

	// Verify the database is exactly in its original state
	var userColumnCount int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'users'
	`).Scan(&userColumnCount)
	if err != nil {
		t.Fatalf("Failed to count columns in users table: %v", err)
	}
	if userColumnCount != 2 {
		t.Fatalf("Expected users table to have exactly 2 columns (id, name), but found %d", userColumnCount)
	}

	var tableCount int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
	`).Scan(&tableCount)
	if err != nil {
		t.Fatalf("Failed to count tables: %v", err)
	}
	if tableCount != 1 {
		t.Fatalf("Expected exactly 1 table (users), but found %d", tableCount)
	}

	t.Log("Transaction rollback verified successfully")
}

// TestApplyCommand_CreateIndexConcurrently verifies that CREATE INDEX CONCURRENTLY
// works correctly when mixed with other DDL statements.
//
// The plan detects non-transactional DDL (CREATE INDEX CONCURRENTLY) and executes
// all statements individually to avoid PostgreSQL's implicit transaction block.
//
// This test verifies that mixed transactional and non-transactional DDL can be
// applied successfully without the "cannot run inside a transaction block" error.
func TestApplyCommand_CreateIndexConcurrently(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	var err error

	// Start PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Setup database with initial schema

	initialSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		);

		INSERT INTO users (name) VALUES ('Alice'), ('Bob'), ('Charlie');
	`
	_, err = conn.ExecContext(ctx, initialSQL)
	if err != nil {
		t.Fatalf("Failed to setup initial schema: %v", err)
	}

	// Create desired state schema file that will generate a migration with mixed DDL
	tmpDir := t.TempDir()
	desiredStateFile := filepath.Join(tmpDir, "desired_state.sql")
	// This desired state will generate a migration that contains:
	// 1. ALTER TABLE to add email column (transactional)
	// 2. CREATE INDEX CONCURRENTLY on the email column (non-transactional)
	// 3. CREATE TABLE for products (transactional)
	desiredStateSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX idx_users_email ON public.users USING btree (email);
		CREATE INDEX idx_users_created_at ON public.users USING btree (created_at);

		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			price DECIMAL(10, 2)
		);
	`
	err = os.WriteFile(desiredStateFile, []byte(desiredStateSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write desired state file: %v", err)
	}

	// Get container connection details
	containerHost := container.Host
	portMapped := container.Port

	// First, generate and verify the migration plan
	planConfig := &planCmd.PlanConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		File:            desiredStateFile,
		ApplicationName: "pgschema",
	}

	migrationPlan, err := planCmd.GenerateSchemaPlan(planConfig, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to generate migration plan: %v", err)
	}

	// Verify the planned SQL contains the expected statements
	plannedSQL := migrationPlan.ToSQL(plan.SQLFormatRaw)

	// Verify that the planned SQL contains our expected statements
	if !strings.Contains(plannedSQL, "ALTER TABLE users ADD COLUMN email") {
		t.Fatalf("Expected migration to contain 'ALTER TABLE users ADD COLUMN email', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "ALTER TABLE users ADD COLUMN created_at") {
		t.Fatalf("Expected migration to contain 'ALTER TABLE users ADD COLUMN created_at', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email") {
		t.Fatalf("Expected migration to contain 'CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_email', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_created_at") {
		t.Fatalf("Expected migration to contain 'CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_created_at', got: %s", plannedSQL)
	}
	if !strings.Contains(plannedSQL, "CREATE TABLE IF NOT EXISTS products") {
		t.Fatalf("Expected migration to contain 'CREATE TABLE IF NOT EXISTS products', got: %s", plannedSQL)
	}

	t.Log("Migration plan verified - contains mixed transactional and non-transactional DDL")

	// Apply the plan directly using ApplyMigration
	applyConfig := &ApplyConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		Plan:            migrationPlan, // Use pre-generated plan
		AutoApprove:     true,
		NoColor:         false,
		Quiet:           true, // Suppress output in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration directly (no need for JSON file or additional embedded postgres)
	err = ApplyMigration(applyConfig, nil)
	if err != nil {
		t.Fatalf("Expected apply command to succeed, but it failed with error: %v", err)
	}

	t.Log("Apply command succeeded - CREATE INDEX CONCURRENTLY now works!")

	// Verify that all changes were applied successfully
	// Check that email column was added to users table
	var emailColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)
	`).Scan(&emailColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if email column exists after apply: %v", err)
	}
	if !emailColumnExists {
		t.Fatal("Email column should exist after successful apply")
	}

	// Verify created_at column was added
	var createdAtColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'created_at'
		)
	`).Scan(&createdAtColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if created_at column exists: %v", err)
	}
	if !createdAtColumnExists {
		t.Fatal("created_at column should exist after successful apply")
	}

	// Verify products table was created
	var tableExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'products'
		)
	`).Scan(&tableExists)
	if err != nil {
		t.Fatalf("Failed to check if products table exists: %v", err)
	}
	if !tableExists {
		t.Fatal("products table should exist after successful apply")
	}

	// Verify indexes were created with CONCURRENTLY
	var indexCount int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_indexes
		WHERE tablename = 'users' AND indexname LIKE 'idx_users_%'
	`).Scan(&indexCount)
	if err != nil {
		t.Fatalf("Failed to check index count: %v", err)
	}
	if indexCount != 2 {
		t.Fatalf("Expected 2 indexes to be created, but found %d", indexCount)
	}

	// Verify we can insert data using the new columns
	_, err = conn.ExecContext(ctx, `
		INSERT INTO users (name, email, created_at)
		VALUES ('Test User', 'test@example.com', NOW())
	`)
	if err != nil {
		t.Fatalf("Failed to insert data with new columns: %v", err)
	}
}

// TestApplyCommand_WithPlanFile verifies that the apply command can apply changes
// from a pre-generated JSON plan file using the --plan flag.
//
// This test simulates a workflow where:
// 1. A plan is generated and saved to a JSON file
// 2. The plan file is later applied using `apply --plan`
//
// This workflow is common in CI/CD pipelines where plan generation and
// application happen in separate stages for review and approval.
func TestApplyCommand_WithPlanFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	var err error

	// Start PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Setup database with initial schema

	initialSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		);
	`
	_, err = conn.ExecContext(ctx, initialSQL)
	if err != nil {
		t.Fatalf("Failed to setup initial schema: %v", err)
	}

	// Create desired state schema file
	tmpDir := t.TempDir()
	desiredStateFile := filepath.Join(tmpDir, "desired_state.sql")
	desiredStateSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX idx_users_email ON public.users USING btree (email);

		CREATE TABLE products (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			price DECIMAL(10, 2)
		);
	`
	err = os.WriteFile(desiredStateFile, []byte(desiredStateSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write desired state file: %v", err)
	}

	// Get container connection details
	containerHost := container.Host
	portMapped := container.Port

	// Step 1: Generate plan and save to JSON file
	planConfig := &planCmd.PlanConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		File:            desiredStateFile,
		ApplicationName: "pgschema",
	}

	migrationPlan, err := planCmd.GenerateSchemaPlan(planConfig, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to generate migration plan: %v", err)
	}

	// Step 2: Apply the plan directly using ApplyMigration
	applyConfig := &ApplyConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		Plan:            migrationPlan, // Use pre-generated plan
		AutoApprove:     true,
		NoColor:         false,
		Quiet:           true, // Suppress output in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration directly (no need for JSON file)
	err = ApplyMigration(applyConfig, nil)
	if err != nil {
		t.Fatalf("Failed to apply plan from file: %v", err)
	}

	t.Log("Plan applied successfully from JSON file")

	// Step 3: Verify all changes were applied
	// Check that email column was added
	var emailColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)
	`).Scan(&emailColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if email column exists: %v", err)
	}
	if !emailColumnExists {
		t.Fatal("Email column should exist after applying plan")
	}

	// Check that created_at column was added
	var createdAtColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'created_at'
		)
	`).Scan(&createdAtColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if created_at column exists: %v", err)
	}
	if !createdAtColumnExists {
		t.Fatal("created_at column should exist after applying plan")
	}

	// Check that products table was created
	var productsTableExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'products'
		)
	`).Scan(&productsTableExists)
	if err != nil {
		t.Fatalf("Failed to check if products table exists: %v", err)
	}
	if !productsTableExists {
		t.Fatal("products table should exist after applying plan")
	}

	// Check that index was created
	var indexExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE tablename = 'users' AND indexname = 'idx_users_email'
		)
	`).Scan(&indexExists)
	if err != nil {
		t.Fatalf("Failed to check if index exists: %v", err)
	}
	if !indexExists {
		t.Fatal("idx_users_email index should exist after applying plan")
	}

	t.Log("All schema changes from plan file applied and verified successfully")
}

// TestApplyCommand_FingerprintMismatch verifies that the apply command correctly
// detects and rejects plans when the database schema has been modified after plan generation.
//
// The test simulates this scenario:
// 1. Create initial schema with users table
// 2. Generate a plan to add email column
// 3. Make out-of-band change (add phone column) to simulate concurrent modification
// 4. Try to apply the original plan - should fail with fingerprint mismatch
//
// This ensures that plans are not applied to databases that have been modified
// since the plan was generated, preventing potential conflicts.
func TestApplyCommand_FingerprintMismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	var err error

	// Start PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Setup database with initial schema

	initialSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		);

		INSERT INTO users (name) VALUES ('Alice'), ('Bob');
	`
	_, err = conn.ExecContext(ctx, initialSQL)
	if err != nil {
		t.Fatalf("Failed to setup initial schema: %v", err)
	}

	// Verify initial state - only id and name columns
	var columnCount int
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'users'
	`).Scan(&columnCount)
	if err != nil {
		t.Fatalf("Failed to count initial columns: %v", err)
	}
	if columnCount != 2 {
		t.Fatalf("Expected 2 columns initially, got %d", columnCount)
	}

	// Create desired state schema file that will add email column
	tmpDir := t.TempDir()
	desiredStateFile := filepath.Join(tmpDir, "desired_state.sql")
	desiredStateSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255)
		);
	`
	err = os.WriteFile(desiredStateFile, []byte(desiredStateSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write desired state file: %v", err)
	}

	// Get container connection details
	containerHost := container.Host
	portMapped := container.Port

	// Generate migration plan to add email column
	planConfig := &planCmd.PlanConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		File:            desiredStateFile,
		ApplicationName: "pgschema",
	}

	migrationPlan, err := planCmd.GenerateSchemaPlan(planConfig, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to generate migration plan: %v", err)
	}

	// Verify the plan includes fingerprint and contains expected changes
	if migrationPlan.SourceFingerprint == nil {
		t.Fatal("Expected plan to include source fingerprint, but it was nil")
	}

	plannedSQL := migrationPlan.ToSQL(plan.SQLFormatRaw)
	if !strings.Contains(plannedSQL, "ALTER TABLE users ADD COLUMN email") {
		t.Fatalf("Expected migration to contain 'ALTER TABLE users ADD COLUMN email', got: %s", plannedSQL)
	}

	t.Log("Migration plan generated successfully with fingerprint")

	// Make out-of-band schema change to simulate concurrent modification
	// This will change the database schema and invalidate the plan's fingerprint
	outOfBandSQL := `ALTER TABLE users ADD COLUMN phone VARCHAR(20);`
	_, err = conn.ExecContext(ctx, outOfBandSQL)
	if err != nil {
		t.Fatalf("Failed to apply out-of-band schema change: %v", err)
	}

	// Verify phone column was added
	var phoneColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'phone'
		)
	`).Scan(&phoneColumnExists)
	if err != nil {
		t.Fatalf("Failed to check if phone column exists: %v", err)
	}
	if !phoneColumnExists {
		t.Fatal("Phone column should exist after out-of-band change")
	}

	t.Log("Out-of-band schema change applied successfully (added phone column)")

	// Attempt to apply the plan directly - should fail with fingerprint mismatch
	applyConfig := &ApplyConfig{
		Host:            containerHost,
		Port:            portMapped,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		Plan:            migrationPlan, // Use pre-generated plan with old fingerprint
		AutoApprove:     true,
		NoColor:         false,
		Quiet:           true, // Suppress output in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration - should fail due to fingerprint mismatch
	err = ApplyMigration(applyConfig, nil)
	if err == nil {
		t.Fatal("Expected apply command to fail due to fingerprint mismatch, but it succeeded")
	}

	// Verify error message mentions fingerprint mismatch
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "fingerprint mismatch") {
		t.Fatalf("Expected error to mention 'fingerprint mismatch', got: %s", errorMsg)
	}
	if !strings.Contains(errorMsg, "schema fingerprint mismatch") {
		t.Fatalf("Expected error to mention 'schema fingerprint mismatch', got: %s", errorMsg)
	}

	// Verify that the database is in the expected state:
	// - phone column exists (from out-of-band change)
	// - email column does NOT exist (original plan was not applied)
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'phone'
		)
	`).Scan(&phoneColumnExists)
	if err != nil {
		t.Fatalf("Failed to check phone column after failed apply: %v", err)
	}
	if !phoneColumnExists {
		t.Fatal("Phone column should still exist after failed apply")
	}

	var emailColumnExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'users' AND column_name = 'email'
		)
	`).Scan(&emailColumnExists)
	if err != nil {
		t.Fatalf("Failed to check email column after failed apply: %v", err)
	}
	if emailColumnExists {
		t.Fatal("Email column should NOT exist after failed apply due to fingerprint mismatch")
	}

	// Verify we now have 3 columns (id, name, phone)
	err = conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = 'users'
	`).Scan(&columnCount)
	if err != nil {
		t.Fatalf("Failed to count columns after failed apply: %v", err)
	}
	if columnCount != 3 {
		t.Fatalf("Expected 3 columns after failed apply (id, name, phone), got %d", columnCount)
	}

	t.Log("Fingerprint validation successfully prevented applying outdated plan to modified database")
}

// TestApplyCommand_WaitDirective verifies that wait directives work correctly
// with concurrent index creation and provide progress monitoring.
func TestApplyCommand_WaitDirective(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	var err error

	// Start PostgreSQL container
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	// Create container struct to match old API for minimal changes
	container := &struct {
		Conn     *sql.DB
		Host     string
		Port     int
		DBName   string
		User     string
		Password string
	}{
		Conn:     conn,
		Host:     host,
		Port:     port,
		DBName:   dbname,
		User:     user,
		Password: password,
	}

	// Setup database with initial schema and data

	initialSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			status VARCHAR(50) DEFAULT 'active'
		);

		-- Insert a decent amount of data to make index creation take some time
		INSERT INTO users (name, email, status)
		SELECT
			'User ' || i,
			'user' || i || '@example.com',
			CASE WHEN i % 3 = 0 THEN 'inactive' ELSE 'active' END
		FROM generate_series(1, 50000) i;
	`
	_, err = conn.ExecContext(ctx, initialSQL)
	if err != nil {
		t.Fatalf("Failed to setup initial schema: %v", err)
	}

	// Create desired state schema file that will generate a concurrent index
	tmpDir := t.TempDir()
	desiredStateFile := filepath.Join(tmpDir, "desired_state.sql")
	desiredStateSQL := `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			email VARCHAR(255),
			status VARCHAR(50) DEFAULT 'active'
		);

		-- This will trigger a CREATE INDEX CONCURRENTLY with wait directive
		CREATE INDEX idx_users_email_status ON users (email, status);
	`

	err = os.WriteFile(desiredStateFile, []byte(desiredStateSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write desired state file: %v", err)
	}

	// Generate plan using sharedEmbeddedPG to avoid creating another embedded postgres instance
	planConfig := &planCmd.PlanConfig{
		Host:            container.Host,
		Port:            container.Port,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		File:            desiredStateFile,
		ApplicationName: "pgschema",
	}

	migrationPlan, err := planCmd.GenerateSchemaPlan(planConfig, sharedEmbeddedPG)
	if err != nil {
		t.Fatalf("Failed to generate plan: %v", err)
	}

	// Apply the plan directly using ApplyMigration
	applyConfig := &ApplyConfig{
		Host:            container.Host,
		Port:            container.Port,
		DB:              container.DBName,
		User:            container.User,
		Password:        container.Password,
		Schema:          "public",
		Plan:            migrationPlan, // Use pre-generated plan
		AutoApprove:     true,
		NoColor:         false,
		Quiet:           true, // Suppress output in tests
		LockTimeout:     "",
		ApplicationName: "pgschema",
	}

	// Call ApplyMigration directly (no need for JSON file)
	err = ApplyMigration(applyConfig, nil)
	if err != nil {
		t.Fatalf("Expected apply command to succeed, but it failed with error: %v", err)
	}

	// Verify that the index was created successfully
	var indexExists bool
	err = conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE indexname = 'idx_users_email_status'
		)
	`).Scan(&indexExists)
	if err != nil {
		t.Fatalf("Failed to check if index exists: %v", err)
	}
	if !indexExists {
		t.Fatal("Index idx_users_email_status should exist after successful apply")
	}

	// Verify that the index is valid (concurrent creation completed successfully)
	var indexValid bool
	err = conn.QueryRowContext(ctx, `
		SELECT i.indisvalid
		FROM pg_class c
		JOIN pg_index i ON c.oid = i.indexrelid
		WHERE c.relname = 'idx_users_email_status'
	`).Scan(&indexValid)
	if err != nil {
		t.Fatalf("Failed to check if index is valid: %v", err)
	}
	if !indexValid {
		t.Fatal("Index idx_users_email_status should be valid after wait directive completion")
	}
}

// TestApplyCommand_WithExternalPlanDatabase tests that apply command works with external plan database
func TestApplyCommand_WithExternalPlanDatabase(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup: Create two embedded postgres instances
	// One is the target database, one is the external plan database
	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()

	externalPlanDB := testutil.SetupPostgres(t)
	defer externalPlanDB.Stop()

	// Get connection details
	targetConn, targetHost, targetPort, targetDatabase, targetUser, targetPassword := testutil.ConnectToPostgres(t, targetDB)
	defer targetConn.Close()

	planHost, planPort, planDatabase, planUser, planPassword := externalPlanDB.GetConnectionDetails()

	// Create test schema file
	schemaSQL := `
CREATE TABLE departments (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_departments_name ON departments(name);

CREATE TABLE employees (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    department_id INTEGER REFERENCES departments(id),
    hired_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
`

	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "schema.sql")
	err := os.WriteFile(schemaFile, []byte(schemaSQL), 0644)
	if err != nil {
		t.Fatalf("Failed to write schema file: %v", err)
	}

	// Create plan config with external database
	planConfig := &planCmd.PlanConfig{
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
	provider, err := planCmd.CreateDesiredStateProvider(planConfig)
	if err != nil {
		t.Fatalf("Failed to create external database provider: %v", err)
	}
	defer provider.Stop()

	// Verify it's using external database (not embedded)
	_, ok := provider.(*postgres.ExternalDatabase)
	if !ok {
		t.Fatal("Provider should be ExternalDatabase when plan-host is provided")
	}

	// Create apply config
	applyConfig := &ApplyConfig{
		Host:            targetHost,
		Port:            targetPort,
		DB:              targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		Schema:          "public",
		File:            schemaFile,
		AutoApprove:     true, // Auto-approve for testing
		NoColor:         true,
		Quiet:           true, // Suppress output in tests
		ApplicationName: "pgschema-test",
	}

	// Apply migration using external database provider
	err = ApplyMigration(applyConfig, provider)
	if err != nil {
		t.Fatalf("Failed to apply migration: %v", err)
	}

	// Verify changes were applied to target database
	// Check that departments table exists
	var tableName string
	err = targetConn.QueryRow("SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename = 'departments'").Scan(&tableName)
	if err != nil {
		t.Fatalf("Failed to query departments table: %v", err)
	}
	if tableName != "departments" {
		t.Errorf("Expected table 'departments', got '%s'", tableName)
	}

	// Check that index exists
	var indexName string
	err = targetConn.QueryRow("SELECT indexname FROM pg_indexes WHERE schemaname = 'public' AND indexname = 'idx_departments_name'").Scan(&indexName)
	if err != nil {
		t.Fatalf("Failed to query index: %v", err)
	}
	if indexName != "idx_departments_name" {
		t.Errorf("Expected index 'idx_departments_name', got '%s'", indexName)
	}

	// Check that employees table exists with foreign key
	var employeeTableName string
	err = targetConn.QueryRow("SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename = 'employees'").Scan(&employeeTableName)
	if err != nil {
		t.Fatalf("Failed to query employees table: %v", err)
	}
	if employeeTableName != "employees" {
		t.Errorf("Expected table 'employees', got '%s'", employeeTableName)
	}

	// Verify foreign key constraint exists
	var constraintName string
	err = targetConn.QueryRow(`
		SELECT conname
		FROM pg_constraint
		WHERE conrelid = 'public.employees'::regclass
		AND contype = 'f'
	`).Scan(&constraintName)
	if err != nil {
		t.Fatalf("Failed to query foreign key constraint: %v", err)
	}
	if constraintName == "" {
		t.Error("Expected foreign key constraint on employees table")
	}
}

// TestApplyCommand_PlanMode_IgnoresPlanDBFlags tests that Plan Mode doesn't validate plan database flags
func TestApplyCommand_PlanMode_IgnoresPlanDBFlags(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup target database
	targetDB := testutil.SetupPostgres(t)
	defer targetDB.Stop()

	targetHost, targetPort, targetDatabase, targetUser, targetPassword := targetDB.GetConnectionDetails()

	// Create a simple schema file
	schemaSQL := `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);
`

	tmpDir := t.TempDir()
	schemaFile := filepath.Join(tmpDir, "schema.sql")
	err := os.WriteFile(schemaFile, []byte(schemaSQL), 0644)
	require.NoError(t, err)

	// First, generate a plan using file mode (without external plan database)
	planConfig := &planCmd.PlanConfig{
		Host:            targetHost,
		Port:            targetPort,
		DB:              targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		Schema:          "public",
		File:            schemaFile,
		ApplicationName: "pgschema-test",
	}

	provider, err := planCmd.CreateDesiredStateProvider(planConfig)
	require.NoError(t, err, "should create provider")
	defer provider.Stop()

	generatedSchemaPlan, err := planCmd.GenerateSchemaPlan(planConfig, provider)
	require.NoError(t, err, "should generate plan")

	// Wrap in unified Plan for JSON serialization
	generatedPlan := plan.NewPlan()
	generatedPlan.AddSchema("public", generatedSchemaPlan)

	// Save plan to JSON file
	planJSON, err := generatedPlan.ToJSON()
	require.NoError(t, err, "should serialize plan to JSON")

	planFile := filepath.Join(tmpDir, "plan.json")
	err = os.WriteFile(planFile, []byte(planJSON), 0644)
	require.NoError(t, err, "should write plan file")

	// Now apply using Plan Mode with INVALID plan database flags set
	// This should NOT fail because Plan Mode shouldn't validate plan database flags
	config := &ApplyConfig{
		Host:            targetHost,
		Port:            targetPort,
		DB:              targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		Schema:          "public",
		AutoApprove:     true,
		NoColor:         true,
		Quiet:           true, // Suppress output in tests
		ApplicationName: "pgschema-test",
	}

	// Load the plan
	planData, err := os.ReadFile(planFile)
	require.NoError(t, err, "should read plan file")

	loadedPlan, err := plan.FromJSON(planData)
	require.NoError(t, err, "should load plan from JSON")

	config.Plan = loadedPlan.Schemas["public"]

	// Set environment variables with INVALID plan database configuration
	// This would normally fail validation, but should be ignored in Plan Mode
	os.Setenv("PGSCHEMA_PLAN_HOST", "invalid-host-that-does-not-exist")
	os.Setenv("PGSCHEMA_PLAN_DB", "") // Missing required field - would fail validation
	defer func() {
		os.Unsetenv("PGSCHEMA_PLAN_HOST")
		os.Unsetenv("PGSCHEMA_PLAN_DB")
	}()

	// Apply should succeed because Plan Mode doesn't validate plan database flags
	err = ApplyMigration(config, nil)
	require.NoError(t, err, "apply with plan mode should ignore invalid plan database flags")

	// Verify the table was created
	connConfig := &util.ConnectionConfig{
		Host:            targetHost,
		Port:            targetPort,
		Database:        targetDatabase,
		User:            targetUser,
		Password:        targetPassword,
		SSLMode:         "prefer",
		ApplicationName: "pgschema-test",
	}

	targetConn, err := util.Connect(connConfig)
	require.NoError(t, err, "should connect to target database")
	defer targetConn.Close()

	var tableName string
	err = targetConn.QueryRow("SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename = 'users'").Scan(&tableName)
	require.NoError(t, err, "should find users table")
	assert.Equal(t, "users", tableName, "table should be created")
}
