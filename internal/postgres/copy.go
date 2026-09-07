package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/include"
)

// ExecuteSchemaSQL runs desired-state SQL that may contain \copy marker
// lines produced by the include processor. Plain SQL runs through
// ExecContext in segments; each marker streams its file into the table with
// COPY ... FROM STDIN, in the position the directive appeared, so rows load
// after the objects they depend on and before anything declared later.
//
// targetSchema is the schema the desired state is written for. A directive
// qualified with it is rewritten to be unqualified so search_path routes the
// rows into the temporary schema, mirroring stripSchemaQualifications.
func ExecuteSchemaSQL(ctx context.Context, conn *sql.Conn, sqlText string, targetSchema string) error {
	if !include.HasCopyMarkers(sqlText) {
		if _, err := util.ExecContextWithLogging(ctx, conn, sqlText, "apply desired state SQL to temporary schema"); err != nil {
			return enhanceApplyError(err, sqlText)
		}
		return nil
	}

	var segment strings.Builder
	flush := func() error {
		text := segment.String()
		segment.Reset()
		if strings.TrimSpace(text) == "" {
			return nil
		}
		if _, err := util.ExecContextWithLogging(ctx, conn, text, "apply desired state SQL to temporary schema"); err != nil {
			return enhanceApplyError(err, text)
		}
		return nil
	}

	for _, line := range strings.Split(sqlText, "\n") {
		directive, ok := include.ParseCopyMarker(line)
		if !ok {
			segment.WriteString(line)
			segment.WriteByte('\n')
			continue
		}
		if err := flush(); err != nil {
			return err
		}
		if err := copyFromFile(ctx, conn, directive, targetSchema); err != nil {
			return err
		}
	}
	return flush()
}

// copyFromFile streams a directive's file into its table.
func copyFromFile(ctx context.Context, conn *sql.Conn, d *include.CopyDirective, targetSchema string) error {
	f, err := os.Open(d.Path)
	if err != nil {
		return fmt.Errorf("failed to open \\copy file for table %s: %w", d.Table, err)
	}
	defer f.Close()

	cmd := buildCopyCommand(d, targetSchema)
	err = WithPgConn(conn, func(pgConn *pgconn.PgConn) error {
		_, err := pgConn.CopyFrom(ctx, f, cmd)
		return err
	})
	if err != nil {
		return fmt.Errorf("failed to load %s into table %s: %w", d.Path, d.Table, err)
	}
	return nil
}

// WithPgConn runs fn with the pgx connection underlying a database/sql
// connection, for protocol-level operations such as COPY that database/sql
// cannot express.
func WithPgConn(conn *sql.Conn, fn func(*pgconn.PgConn) error) error {
	return conn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection type %T", driverConn)
		}
		return fn(pgxConn.Conn().PgConn())
	})
}

// buildCopyCommand renders the server-side COPY for a directive.
func buildCopyCommand(d *include.CopyDirective, targetSchema string) string {
	var b strings.Builder
	b.WriteString("COPY ")
	if d.Schema != "" && d.Schema != targetSchema {
		b.WriteString(quoteIdent(d.Schema))
		b.WriteByte('.')
	}
	b.WriteString(quoteIdent(d.Table))
	if d.Columns != "" {
		b.WriteString(" (")
		b.WriteString(d.Columns)
		b.WriteString(")")
	}
	b.WriteString(" FROM STDIN")
	if d.Options != "" {
		b.WriteByte(' ')
		b.WriteString(d.Options)
	}
	return b.String()
}
