package util

import (
	"context"
	"database/sql"

	"github.com/pgplex/pgschema/internal/logger"
)

// execer is an interface satisfied by both *sql.DB and *sql.Conn,
// allowing ExecContextWithLogging to work with either.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ExecContextWithLogging executes SQL with debug logging if debug mode is enabled.
// It logs the SQL statement before execution and the result/error after execution.
// It accepts both *sql.DB and *sql.Conn via the execer interface.
func ExecContextWithLogging(ctx context.Context, db execer, sqlStmt string, description string) (sql.Result, error) {
	isDebug := logger.IsDebug()
	if isDebug {
		logger.Get().Debug("Executing SQL", "description", description, "sql", sqlStmt)
	}

	result, err := db.ExecContext(ctx, sqlStmt)

	if isDebug {
		if err != nil {
			logger.Get().Debug("SQL execution failed", "description", description, "error", err)
		} else {
			logger.Get().Debug("SQL execution succeeded", "description", description)
		}
	}

	return result, err
}
