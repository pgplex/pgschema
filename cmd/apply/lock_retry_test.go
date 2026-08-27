package apply

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeResult is a minimal sql.Result implementation for tests.
type fakeResult struct{}

func (fakeResult) LastInsertId() (int64, error) { return 0, nil }
func (fakeResult) RowsAffected() (int64, error) { return 0, nil }

// fakeExecer simulates a database connection whose first failN calls fail with
// failErr before succeeding.
type fakeExecer struct {
	calls   int
	failN   int
	failErr error
}

func (f *fakeExecer) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.calls++
	if f.calls <= f.failN {
		return nil, f.failErr
	}
	return fakeResult{}, nil
}

func TestIsLockTimeoutError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"lock timeout pgerror", &pgconn.PgError{Code: lockNotAvailableSQLState}, true},
		{"wrapped lock timeout pgerror", fmt.Errorf("exec failed: %w", &pgconn.PgError{Code: lockNotAvailableSQLState}), true},
		{"other pgerror", &pgconn.PgError{Code: "42601"}, false},
		{"non pgerror", errors.New("some other error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLockTimeoutError(tt.err); got != tt.want {
				t.Errorf("isLockTimeoutError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestExecWithLockRetry_SucceedsAfterRetries(t *testing.T) {
	fe := &fakeExecer{failN: 2, failErr: &pgconn.PgError{Code: lockNotAvailableSQLState}}
	retry := lockRetryConfig{MaxRetries: 3, Backoff: time.Millisecond}

	_, err := execWithLockRetry(context.Background(), fe, "SELECT 1", "test", retry, true)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if fe.calls != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", fe.calls)
	}
}

func TestExecWithLockRetry_ExhaustsRetries(t *testing.T) {
	pgErr := &pgconn.PgError{Code: lockNotAvailableSQLState}
	fe := &fakeExecer{failN: 100, failErr: pgErr}
	retry := lockRetryConfig{MaxRetries: 2, Backoff: time.Millisecond}

	_, err := execWithLockRetry(context.Background(), fe, "SELECT 1", "test", retry, true)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if !errors.Is(err, error(pgErr)) && !isLockTimeoutError(err) {
		t.Errorf("expected the underlying lock timeout error to be returned, got: %v", err)
	}
	if fe.calls != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", fe.calls)
	}
}

func TestExecWithLockRetry_NonRetryableErrorFailsImmediately(t *testing.T) {
	otherErr := &pgconn.PgError{Code: "42601"} // syntax_error
	fe := &fakeExecer{failN: 100, failErr: otherErr}
	retry := lockRetryConfig{MaxRetries: 5, Backoff: time.Millisecond}

	_, err := execWithLockRetry(context.Background(), fe, "SELECT 1", "test", retry, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if fe.calls != 1 {
		t.Errorf("expected 1 call since non-retryable errors should not retry, got %d", fe.calls)
	}
}

func TestExecWithLockRetry_NoRetriesConfigured(t *testing.T) {
	pgErr := &pgconn.PgError{Code: lockNotAvailableSQLState}
	fe := &fakeExecer{failN: 1, failErr: pgErr}
	retry := lockRetryConfig{MaxRetries: 0, Backoff: time.Millisecond}

	_, err := execWithLockRetry(context.Background(), fe, "SELECT 1", "test", retry, true)
	if err == nil {
		t.Fatal("expected error since MaxRetries is 0, got nil")
	}
	if fe.calls != 1 {
		t.Errorf("expected exactly 1 call with MaxRetries=0, got %d", fe.calls)
	}
}

func TestExecWithLockRetry_ContextCancelledDuringBackoff(t *testing.T) {
	pgErr := &pgconn.PgError{Code: lockNotAvailableSQLState}
	fe := &fakeExecer{failN: 100, failErr: pgErr}
	retry := lockRetryConfig{MaxRetries: 5, Backoff: 50 * time.Millisecond}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := execWithLockRetry(ctx, fe, "SELECT 1", "test", retry, true)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if fe.calls != 1 {
		t.Errorf("expected exactly 1 call before cancellation, got %d", fe.calls)
	}
}

// TestApplyMigration_ValidatesLockRetryConfig verifies that ApplyMigration itself
// rejects inconsistent lock retry settings, not just the RunApply CLI entry point.
// This matters for callers that build an ApplyConfig directly and skip RunApply's
// flag validation entirely.
func TestApplyMigration_ValidatesLockRetryConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *ApplyConfig
		wantErr string
	}{
		{
			name:    "negative retries",
			config:  &ApplyConfig{LockRetries: -1},
			wantErr: "lock timeout retries must be non-negative",
		},
		{
			name:    "retries without lock timeout",
			config:  &ApplyConfig{LockRetries: 3, LockRetryWait: time.Second},
			wantErr: "lock timeout retries require a lock timeout",
		},
		{
			name:    "retries with non-positive retry wait",
			config:  &ApplyConfig{LockRetries: 3, LockTimeout: "5s", LockRetryWait: 0},
			wantErr: "lock timeout retry wait must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyMigration(tt.config, nil)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}
