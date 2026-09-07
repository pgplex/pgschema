package dump

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// DataDir is the directory, beside the main output file, that holds one CSV
// file per reference table.
const DataDir = "data"

// DataTables returns the data-managed tables of a schema, parents before the
// children that reference them (and by name otherwise), which is the order
// their rows must load in.
func DataTables(schemaIR *ir.IR, schemaName string) []*ir.Table {
	dbSchema, ok := schemaIR.Schemas[schemaName]
	if !ok {
		return nil
	}
	var tables []*ir.Table
	for _, name := range dbSchema.TableNames() {
		if table := dbSchema.Tables[name]; table.DataManaged {
			tables = append(tables, table)
		}
	}
	return diff.TopologicallySortTables(tables)
}

// DataFileName returns the CSV path of a table relative to the main output file.
func (f *DumpFormatter) DataFileName(table *ir.Table) string {
	return filepath.ToSlash(filepath.Join(DataDir, f.sanitizeFileName(table.Name)+".csv"))
}

// WriteDataFiles writes data/<table>.csv beside outputPath for every table,
// using PostgreSQL's own CSV writer so quoting and NULL handling are exactly
// what COPY ... FROM reads back. Rows are ordered by primary key.
func (f *DumpFormatter) WriteDataFiles(ctx context.Context, db *sql.DB, tables []*ir.Table, outputPath string) error {
	if len(tables) == 0 {
		return nil
	}
	baseDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(filepath.Join(baseDir, DataDir), 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Close()

	// A transaction scopes the SET LOCAL settings to this dump.
	if _, err := conn.ExecContext(ctx, "BEGIN READ ONLY"); err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer conn.ExecContext(ctx, "ROLLBACK")
	if _, err := conn.ExecContext(ctx, ir.DataSessionSettings); err != nil {
		return fmt.Errorf("failed to set session settings: %w", err)
	}

	for _, table := range tables {
		path := filepath.Join(baseDir, filepath.FromSlash(f.DataFileName(table)))
		if err := writeCSV(ctx, conn, table, path); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return nil
}

func writeCSV(ctx context.Context, conn *sql.Conn, table *ir.Table, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	cmd := fmt.Sprintf("COPY (%s) TO STDOUT WITH (FORMAT csv, HEADER)", ir.RowQuery(table))
	return postgres.WithPgConn(conn, func(pgConn *pgconn.PgConn) error {
		_, err := pgConn.CopyTo(ctx, file, cmd)
		return err
	})
}

// FormatDataDirectives renders the \copy directive of every table, each under
// a TABLE DATA comment header, for appending after all other objects.
func (f *DumpFormatter) FormatDataDirectives(tables []*ir.Table) string {
	var out strings.Builder
	for _, table := range tables {
		out.WriteString("\n")
		if !f.noComments {
			out.WriteString(commentHeader(table.Name, "TABLE DATA", f.getCommentSchemaName(table.Schema+"."+table.Name)))
		}
		out.WriteString(f.copyDirective(table))
		out.WriteString("\n")
	}
	return out.String()
}

func (f *DumpFormatter) copyDirective(table *ir.Table) string {
	cols := table.DataColumns()
	names := make([]string, len(cols))
	for i, col := range cols {
		names[i] = ir.QuoteIdentifier(col.Name)
	}
	name := ir.QualifyEntityNameWithQuotesMode(table.Schema, table.Name, f.targetSchema, f.qualifySchema)
	return fmt.Sprintf("\\copy %s (%s) FROM '%s' WITH (FORMAT csv, HEADER)", name, strings.Join(names, ", "), f.DataFileName(table))
}
