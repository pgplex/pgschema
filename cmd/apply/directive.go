package apply

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pgplex/pgschema/internal/plan"
)

const (
	// Progressive intervals for wait directive polling
	waitDirectiveShortInterval  = 1 * time.Second  // For first few checks (up to 10s)
	waitDirectiveMediumInterval = 5 * time.Second  // For medium duration (10-30s)
	waitDirectiveLongInterval   = 10 * time.Second // For long operations (after 30s)

	// Thresholds for switching intervals
	waitDirectiveShortDuration  = 10 * time.Second // Use 1s interval up to 10s
	waitDirectiveMediumDuration = 30 * time.Second // Use 5s interval up to 30s
	// After 30s, use 10s interval
)

// executeDirective executes a directive based on its type
type directiveExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func executeDirective(ctx context.Context, conn directiveExecer, directive *plan.Directive, query string) error {
	switch directive.Type {
	case "wait":
		return executeWaitDirective(ctx, conn, directive, query)
	default:
		return fmt.Errorf("unknown directive type: %s", directive.Type)
	}
}

// checkWaitStatus executes the wait query and extracts done/progress values
func checkWaitStatus(ctx context.Context, conn directiveExecer, query string) (done bool, progress int, err error) {
	rows, err := conn.QueryContext(ctx, query)
	if err != nil {
		return false, -1, fmt.Errorf("failed to execute wait query: %w", err)
	}
	defer rows.Close()

	// Get column names to validate expected columns exist
	columns, err := rows.Columns()
	if err != nil {
		return false, -1, fmt.Errorf("failed to get query columns: %w", err)
	}

	// Validate required "done" column exists
	doneColumnIndex := -1
	progressColumnIndex := -1
	for i, col := range columns {
		switch col {
		case "done":
			doneColumnIndex = i
		case "progress":
			progressColumnIndex = i
		}
	}

	if doneColumnIndex == -1 {
		return false, -1, fmt.Errorf("wait directive query must return a 'done' column")
	}

	// Prepare values slice for scanning
	values := make([]any, len(columns))
	scanArgs := make([]any, len(columns))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	// Scan the first row
	if !rows.Next() {
		return false, -1, fmt.Errorf("wait directive query returned no rows")
	}

	err = rows.Scan(scanArgs...)
	if err != nil {
		return false, -1, fmt.Errorf("failed to scan wait query result: %w", err)
	}

	// Extract done value (required)
	doneValue, ok := values[doneColumnIndex].(bool)
	if !ok {
		return false, -1, fmt.Errorf("wait directive 'done' column must be boolean, got %T", values[doneColumnIndex])
	}

	// Extract progress value (optional)
	var progressValue int = -1
	if progressColumnIndex != -1 {
		if prog, ok := values[progressColumnIndex].(int64); ok {
			progressValue = int(prog)
		} else if prog, ok := values[progressColumnIndex].(int32); ok {
			progressValue = int(prog)
		}
	}

	return doneValue, progressValue, nil
}

// executeWaitDirective monitors a long-running operation until completion
func executeWaitDirective(ctx context.Context, conn directiveExecer, directive *plan.Directive, query string) error {
	if directive.Message != "" {
		fmt.Printf("  Waiting: %s\n", directive.Message)
	} else {
		fmt.Printf("  Waiting for operation to complete...\n")
	}

	startTime := time.Now()
	lastProgress := -1

	// Immediate check for fast operations
	done, progress, err := checkWaitStatus(ctx, conn, query)
	if err != nil {
		return err
	}

	if done {
		elapsed := time.Since(startTime)
		if elapsed < 1*time.Second {
			fmt.Printf("    Completed immediately (< 1s)\n")
		} else {
			fmt.Printf("    Completed successfully (%v)\n", elapsed.Round(time.Millisecond))
		}
		return nil
	}

	// Report initial progress if available
	if progress >= 0 && progress != lastProgress {
		fmt.Printf("    Progress: %d%%\n", progress)
		lastProgress = progress
	}

	// Progressive polling for longer operations
	for {
		// Determine next poll interval based on elapsed time
		elapsed := time.Since(startTime)
		var interval time.Duration

		switch {
		case elapsed < waitDirectiveShortDuration:
			interval = waitDirectiveShortInterval // 1s for first 10s
		case elapsed < waitDirectiveMediumDuration:
			interval = waitDirectiveMediumInterval // 5s for 10-30s
		default:
			interval = waitDirectiveLongInterval // 10s after 30s
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			done, progress, err = checkWaitStatus(ctx, conn, query)
			if err != nil {
				return err
			}

			// Update progress if changed
			if progress >= 0 && progress != lastProgress {
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("    Progress: %d%% (elapsed: %v)\n", progress, elapsed)
				lastProgress = progress
			}

			// Check if complete
			if done {
				elapsed := time.Since(startTime).Round(time.Second)
				fmt.Printf("    Completed successfully (total time: %v)\n", elapsed)
				return nil
			}
		}
	}
}
