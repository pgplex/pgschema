package ir

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// DataConfig lists the reference tables whose rows pgschema manages. It is the
// [data] section of pgschema.toml. Patterns follow the same glob and "!"
// negation rules as .pgschemaignore and match unqualified table names.
type DataConfig struct {
	Tables []string `toml:"tables,omitempty"`
}

// IsDataTable reports whether a table's rows are managed.
func (c *DataConfig) IsDataTable(tableName string) bool {
	if c == nil {
		return false
	}
	return matchPatterns(tableName, c.Tables)
}

// Row is one row of a data-managed table. Values are in the order of the
// table's non-generated columns (see Table.DataColumns); nil is SQL NULL.
// Values hold PostgreSQL's canonical text form for the column type.
type Row struct {
	Values []*string
}

// DataColumns returns the columns that carry row data, in table order:
// every column except generated ones.
func (t *Table) DataColumns() []*Column {
	cols := make([]*Column, 0, len(t.Columns))
	for _, col := range t.Columns {
		if col.IsGenerated {
			continue
		}
		cols = append(cols, col)
	}
	sort.SliceStable(cols, func(a, b int) bool { return cols[a].Position < cols[b].Position })
	return cols
}

// PrimaryKeyColumns returns the names of the primary key columns in key order,
// or nil when the table has no primary key.
func (t *Table) PrimaryKeyColumns() []string {
	for _, c := range t.Constraints {
		if c.Type != ConstraintTypePrimaryKey {
			continue
		}
		cols := make([]*ConstraintColumn, len(c.Columns))
		copy(cols, c.Columns)
		sort.SliceStable(cols, func(a, b int) bool { return cols[a].Position < cols[b].Position })
		names := make([]string, len(cols))
		for i, cc := range cols {
			names[i] = cc.Name
		}
		return names
	}
	return nil
}

// DataSessionSettings pins the text output format of every type whose
// rendering depends on session state, so rows read from the plan database,
// from the target database, and by dump all use the same canonical form.
// Run inside a transaction; SET LOCAL reverts at its end.
const DataSessionSettings = `SET LOCAL TimeZone = 'UTC';
SET LOCAL DateStyle = 'ISO, MDY';
SET LOCAL IntervalStyle = 'postgres';
SET LOCAL bytea_output = 'hex';
SET LOCAL extra_float_digits = 3`

// buildRows marks every data-managed table and, unless the inspector was
// told to skip rows, loads their rows. It runs after tables, columns,
// constraints, and partitions are known. A managed table without a primary
// key is an error, since rows are matched by primary key.
func (i *Inspector) buildRows(ctx context.Context, schema *IR, targetSchema string) error {
	if i.dataConfig == nil || len(i.dataConfig.Tables) == 0 {
		return nil
	}
	dbSchema, ok := schema.Schemas[targetSchema]
	if !ok {
		return nil
	}

	var managed []*Table
	for _, name := range dbSchema.TableNames() {
		table := dbSchema.Tables[name]
		if !i.dataConfig.IsDataTable(name) {
			continue
		}
		// Partition children are managed through their parent, which sees
		// every row.
		if table.PartitionOf != "" || table.IsExternal {
			continue
		}
		if len(table.PrimaryKeyColumns()) == 0 {
			return fmt.Errorf("table %q is a data table but has no primary key; rows are matched by primary key", name)
		}
		table.DataManaged = true
		managed = append(managed, table)
	}
	if len(managed) == 0 || !i.loadRows {
		return nil
	}

	// A transaction scopes the SET LOCAL settings to this read so a pooled
	// connection is returned unchanged.
	tx, err := i.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("failed to begin transaction for reading rows: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, DataSessionSettings); err != nil {
		return fmt.Errorf("failed to set session settings for reading rows: %w", err)
	}

	for _, table := range managed {
		rows, err := readTableRows(ctx, tx, table)
		if err != nil {
			return fmt.Errorf("failed to read rows of table %q: %w", table.Name, err)
		}
		table.Rows = rows
	}
	return nil
}

// RowQuery returns the SELECT that reads a table's data columns as text,
// ordered by primary key. It is shared by the inspector and by dump.
func RowQuery(table *Table) string {
	cols := table.DataColumns()
	selects := make([]string, len(cols))
	for idx, col := range cols {
		selects[idx] = QuoteIdentifier(col.Name) + "::text"
	}
	pk := table.PrimaryKeyColumns()
	orderBy := make([]string, len(pk))
	for idx, name := range pk {
		orderBy[idx] = QuoteIdentifier(name)
	}
	return fmt.Sprintf("SELECT %s FROM %s.%s ORDER BY %s",
		strings.Join(selects, ", "),
		QuoteIdentifier(table.Schema), QuoteIdentifier(table.Name),
		strings.Join(orderBy, ", "))
}

func readTableRows(ctx context.Context, tx *sql.Tx, table *Table) ([]*Row, error) {
	cols := table.DataColumns()
	result := []*Row{}
	if len(cols) == 0 {
		return result, nil
	}

	rs, err := tx.QueryContext(ctx, RowQuery(table))
	if err != nil {
		return nil, err
	}
	defer rs.Close()

	for rs.Next() {
		scanned := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for idx := range scanned {
			ptrs[idx] = &scanned[idx]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := &Row{Values: make([]*string, len(cols))}
		for idx, v := range scanned {
			if v.Valid {
				s := v.String
				row.Values[idx] = &s
			}
		}
		result = append(result, row)
	}
	return result, rs.Err()
}

// TableNames returns the schema's table names in sorted order.
func (s *Schema) TableNames() []string {
	names := make([]string, 0, len(s.Tables))
	for name := range s.Tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
