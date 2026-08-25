package apply

import (
	"context"
	"testing"
	"time"

	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/testutil"
)

// lockTestPlan builds a minimal single-statement migration plan for exercising
// lock timeout retries directly via ApplyMigration, bypassing plan generation.
func lockTestPlan(sql string) *plan.Plan {
	return &plan.Plan{
		Groups: []plan.ExecutionGroup{
			{Steps: []plan.Step{{SQL: sql, Type: "table", Operation: "alter", Path: "public.locktest"}}},
		},
	}
}

// TestApplyCommand_LockTimeoutRetrySucceeds verifies that a statement blocked by a
// concurrent lock eventually succeeds once --lock-timeout-retries are exhausted-but-one:
// the lock is released while retries are still available.
func TestApplyCommand_LockTimeoutRetrySucceeds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "CREATE TABLE locktest (id INT)"); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// Hold an ACCESS EXCLUSIVE lock on locktest from a separate session, long enough
	// that the first couple of attempts time out, then release it so a later retry succeeds.
	lockConn, err := conn.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to get dedicated connection: %v", err)
	}
	defer lockConn.Close()

	if _, err := lockConn.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatalf("failed to begin locking transaction: %v", err)
	}
	if _, err := lockConn.ExecContext(ctx, "LOCK TABLE locktest IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("failed to lock table: %v", err)
	}

	const holdDuration = 1200 * time.Millisecond
	released := make(chan struct{})
	go func() {
		defer close(released)
		time.Sleep(holdDuration)
		if _, err := lockConn.ExecContext(context.Background(), "COMMIT"); err != nil {
			t.Errorf("failed to release lock: %v", err)
		}
	}()
	defer func() { <-released }()

	applyConfig := &ApplyConfig{
		Host:          host,
		Port:          port,
		DB:            dbname,
		User:          user,
		Password:      password,
		Schema:        "public",
		Plan:          lockTestPlan("ALTER TABLE locktest ADD COLUMN name TEXT;"),
		AutoApprove:   true,
		Quiet:         true,
		LockTimeout:   "100ms",
		LockRetries:   6,
		LockRetryWait: 250 * time.Millisecond,
	}

	start := time.Now()
	err = ApplyMigration(applyConfig, nil)
	if err != nil {
		t.Fatalf("expected apply to eventually succeed after the lock was released, got error: %v", err)
	}
	t.Logf("apply succeeded after %v", time.Since(start))

	var nameColumnExists bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'locktest' AND column_name = 'name'
		)
	`).Scan(&nameColumnExists); err != nil {
		t.Fatalf("failed to check column existence: %v", err)
	}
	if !nameColumnExists {
		t.Fatal("expected 'name' column to exist after apply succeeded")
	}
}

// TestApplyCommand_LockTimeoutRetryExhausted verifies that apply fails with the
// underlying lock timeout error once all configured retries are exhausted.
func TestApplyCommand_LockTimeoutRetryExhausted(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	embeddedPG := testutil.SetupPostgres(t)
	defer embeddedPG.Stop()
	conn, host, port, dbname, user, password := testutil.ConnectToPostgres(t, embeddedPG)
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "CREATE TABLE locktest (id INT)"); err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// Hold the lock for the entire test - long enough to outlast every retry attempt.
	lockConn, err := conn.Conn(ctx)
	if err != nil {
		t.Fatalf("failed to get dedicated connection: %v", err)
	}
	defer lockConn.Close()

	if _, err := lockConn.ExecContext(ctx, "BEGIN"); err != nil {
		t.Fatalf("failed to begin locking transaction: %v", err)
	}
	if _, err := lockConn.ExecContext(ctx, "LOCK TABLE locktest IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatalf("failed to lock table: %v", err)
	}
	defer lockConn.ExecContext(context.Background(), "ROLLBACK")

	applyConfig := &ApplyConfig{
		Host:          host,
		Port:          port,
		DB:            dbname,
		User:          user,
		Password:      password,
		Schema:        "public",
		Plan:          lockTestPlan("ALTER TABLE locktest ADD COLUMN name TEXT;"),
		AutoApprove:   true,
		Quiet:         true,
		LockTimeout:   "100ms",
		LockRetries:   2,
		LockRetryWait: 50 * time.Millisecond,
	}

	err = ApplyMigration(applyConfig, nil)
	if err == nil {
		t.Fatal("expected apply to fail once retries are exhausted while lock is still held")
	}
	if !isLockTimeoutError(err) {
		t.Errorf("expected the returned error to wrap a lock timeout (55P03), got: %v", err)
	}

	var nameColumnExists bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'locktest' AND column_name = 'name'
		)
	`).Scan(&nameColumnExists); err != nil {
		t.Fatalf("failed to check column existence: %v", err)
	}
	if nameColumnExists {
		t.Fatal("expected 'name' column to NOT exist since apply should have failed")
	}
}
