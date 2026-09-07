package diff

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// tableDataDiff holds the row changes of one data-managed table.
type tableDataDiff struct {
	Table   *ir.Table      // desired table
	Columns []*ir.Column   // desired data columns, the order of every Row here
	colIdx  map[string]int // column name -> index in Columns
	pkIdx   []int          // indexes of primary key columns within Columns
	Inserts []*ir.Row
	Updates []*rowUpdate
	Deletes []*rowDelete
	// DeleteAll replaces Deletes when the primary key columns changed in this
	// migration: current rows cannot be addressed by a key that the DDL may
	// have already dropped, so the whole table is cleared before inserts.
	DeleteAll bool
}

type rowUpdate struct {
	Key     string
	Row     *ir.Row // desired values
	Changed []int   // indexes into Columns whose value differs
}

type rowDelete struct {
	Key       string
	PKColumns []string  // primary key column names of the current table
	PKValues  []*string // their values in the current row
}

// dataRow is the DiffSource of one row change; the object name is the
// primary key, so the plan lists rows under their table by key.
type dataRow struct {
	key string
}

func (r *dataRow) GetObjectName() string { return r.key }

// keySeparator joins primary key values internally; keyDisplaySeparator joins
// them for paths and plan output.
const (
	keySeparator        = "\x00"
	keyDisplaySeparator = ","
)

// diffTableData compares the rows of every data-managed desired table with
// the current table of the same name. Rows are matched by primary key.
func diffTableData(oldTables, newTables map[string]*ir.Table) []*tableDataDiff {
	var result []*tableDataDiff
	for _, key := range sortedKeys(newTables) {
		newTable := newTables[key]
		if !newTable.DataManaged || newTable.IsExternal {
			continue
		}
		oldTable := oldTables[key]
		if d := diffRows(oldTable, newTable); d != nil {
			result = append(result, d)
		}
	}
	return result
}

func diffRows(oldTable, newTable *ir.Table) *tableDataDiff {
	d := &tableDataDiff{
		Table:   newTable,
		Columns: newTable.DataColumns(),
	}
	d.colIdx = columnIndex(d.Columns)
	newPK := newTable.PrimaryKeyColumns()
	d.pkIdx = pkIndexes(d.colIdx, newPK)

	newKeys := make(map[string]*ir.Row, len(newTable.Rows))
	var newOrder []string
	for _, row := range newTable.Rows {
		k := rowKey(row, d.pkIdx)
		newKeys[k] = row
		newOrder = append(newOrder, k)
	}

	// Current rows, keyed by the current table's primary key. Keys are only
	// comparable when both tables have the same primary key columns; otherwise
	// every current row is removed and every desired row inserted.
	var oldKeys map[string]*ir.Row
	var oldOrder []string
	var oldColIdx map[string]int
	var oldPK []string
	sameKey := false
	if oldTable != nil {
		oldColIdx = columnIndex(oldTable.DataColumns())
		oldPK = oldTable.PrimaryKeyColumns()
		sameKey = slices.Equal(oldPK, newPK)
		oldPKIdx := pkIndexes(oldColIdx, oldPK)
		oldKeys = make(map[string]*ir.Row, len(oldTable.Rows))
		for _, row := range oldTable.Rows {
			k := rowKey(row, oldPKIdx)
			oldKeys[k] = row
			oldOrder = append(oldOrder, k)
		}
	}

	if oldTable != nil && !sameKey && len(oldTable.Rows) > 0 {
		d.DeleteAll = true
	}

	for _, k := range oldOrder {
		oldRow := oldKeys[k]
		if newRow, ok := newKeys[k]; ok && sameKey {
			var changed []int
			for i, col := range d.Columns {
				var oldVal *string
				if oi, ok := oldColIdx[col.Name]; ok {
					oldVal = oldRow.Values[oi]
				}
				if !equalValue(oldVal, newRow.Values[i]) {
					changed = append(changed, i)
				}
			}
			if len(changed) > 0 {
				d.Updates = append(d.Updates, &rowUpdate{Key: k, Row: newRow, Changed: changed})
			}
			continue
		}
		if d.DeleteAll {
			continue
		}
		del := &rowDelete{Key: k, PKColumns: oldPK}
		for _, name := range oldPK {
			del.PKValues = append(del.PKValues, oldRow.Values[oldColIdx[name]])
		}
		d.Deletes = append(d.Deletes, del)
	}
	for _, k := range newOrder {
		if _, ok := oldKeys[k]; ok && sameKey {
			continue
		}
		d.Inserts = append(d.Inserts, newKeys[k])
	}

	if len(d.Inserts) == 0 && len(d.Updates) == 0 && len(d.Deletes) == 0 && !d.DeleteAll {
		return nil
	}
	return d
}

func columnIndex(cols []*ir.Column) map[string]int {
	idx := make(map[string]int, len(cols))
	for i, col := range cols {
		idx[col.Name] = i
	}
	return idx
}

func pkIndexes(colIdx map[string]int, pk []string) []int {
	out := make([]int, len(pk))
	for i, name := range pk {
		out[i] = colIdx[name]
	}
	return out
}

func rowKey(row *ir.Row, pkIdx []int) string {
	parts := make([]string, len(pkIdx))
	for i, idx := range pkIdx {
		if v := row.Values[idx]; v != nil {
			parts[i] = *v
		}
	}
	return strings.Join(parts, keySeparator)
}

// displayKey renders a key for paths and plan output. A composite key is
// joined with commas; a value containing a comma or a quote is quoted CSV
// style so distinct keys never render the same.
func displayKey(key string) string {
	parts := strings.Split(key, keySeparator)
	for i, part := range parts {
		if strings.ContainsAny(part, keyDisplaySeparator+`"`) {
			parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
		}
	}
	return strings.Join(parts, keyDisplaySeparator)
}

// allRowsKey names the step that clears a table whose primary key changed.
const allRowsKey = "*"

func equalValue(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// generateDataSQL emits one statement per row change. Deletes run first,
// child tables before parents; then inserts, parents before children; then
// updates. Every statement runs inside the migration transaction.
func generateDataSQL(diffs []*tableDataDiff, targetSchema string, collector *diffCollector) {
	if len(diffs) == 0 {
		return
	}
	byTable := make(map[*ir.Table]*tableDataDiff, len(diffs))
	tables := make([]*ir.Table, 0, len(diffs))
	for _, d := range diffs {
		byTable[d.Table] = d
		tables = append(tables, d.Table)
	}
	ordered := make([]*tableDataDiff, 0, len(diffs))
	for _, table := range topologicallySortTables(tables) {
		ordered = append(ordered, byTable[table])
	}
	name := func(d *tableDataDiff) string {
		return getTableNameWithSchema(d.Table.Schema, d.Table.Name, targetSchema)
	}

	for _, d := range reverseSlice(ordered) {
		if d.DeleteAll {
			collector.collect(d.context(DiffOperationDrop, allRowsKey), fmt.Sprintf("DELETE FROM %s;", name(d)))
			continue
		}
		for _, del := range d.Deletes {
			where := make([]string, len(del.PKColumns))
			for i, col := range del.PKColumns {
				var column *ir.Column
				if ci, ok := d.colIdx[col]; ok {
					column = d.Columns[ci]
				}
				where[i] = fmt.Sprintf("%s = %s", ir.QuoteIdentifier(col), formatDataLiteral(column, del.PKValues[i]))
			}
			sql := fmt.Sprintf("DELETE FROM %s WHERE %s;", name(d), strings.Join(where, " AND "))
			collector.collect(d.context(DiffOperationDrop, del.Key), sql)
		}
	}

	for _, d := range ordered {
		colNames := make([]string, len(d.Columns))
		overriding := ""
		for i, col := range d.Columns {
			colNames[i] = ir.QuoteIdentifier(col.Name)
			if col.Identity != nil && col.Identity.Generation == "ALWAYS" {
				overriding = " OVERRIDING SYSTEM VALUE"
			}
		}
		for _, row := range d.Inserts {
			values := make([]string, len(d.Columns))
			for i, col := range d.Columns {
				values[i] = formatDataLiteral(col, row.Values[i])
			}
			sql := fmt.Sprintf("INSERT INTO %s (%s)%s VALUES (%s);", name(d), strings.Join(colNames, ", "), overriding, strings.Join(values, ", "))
			collector.collect(d.context(DiffOperationCreate, rowKey(row, d.pkIdx)), sql)
		}
	}

	for _, d := range ordered {
		for _, upd := range d.Updates {
			sets := make([]string, len(upd.Changed))
			for i, ci := range upd.Changed {
				col := d.Columns[ci]
				sets[i] = fmt.Sprintf("%s = %s", ir.QuoteIdentifier(col.Name), formatDataLiteral(col, upd.Row.Values[ci]))
			}
			where := make([]string, len(d.pkIdx))
			for i, pi := range d.pkIdx {
				col := d.Columns[pi]
				where[i] = fmt.Sprintf("%s = %s", ir.QuoteIdentifier(col.Name), formatDataLiteral(col, upd.Row.Values[pi]))
			}
			sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s;", name(d), strings.Join(sets, ", "), strings.Join(where, " AND "))
			collector.collect(d.context(DiffOperationAlter, upd.Key), sql)
		}
	}
}

func (d *tableDataDiff) context(op DiffOperation, key string) *diffContext {
	return &diffContext{
		Type:                DiffTypeTableData,
		Operation:           op,
		Path:                fmt.Sprintf("%s.%s.%s", d.Table.Schema, d.Table.Name, displayKey(key)),
		Source:              &dataRow{key: displayKey(key)},
		CanRunInTransaction: true,
	}
}

var plainNumber = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?([eE][-+]?[0-9]+)?$`)

// formatDataLiteral renders a canonical text value as a SQL literal. Numbers
// and booleans are written bare; everything else is a quoted string that
// PostgreSQL casts to the column type.
func formatDataLiteral(col *ir.Column, v *string) string {
	if v == nil {
		return "NULL"
	}
	if col != nil {
		base := col.DataType
		if i := strings.Index(base, "("); i >= 0 {
			base = base[:i]
		}
		switch strings.ToLower(strings.TrimSpace(base)) {
		case "smallint", "integer", "bigint", "numeric", "decimal", "real", "double precision",
			"int2", "int4", "int8", "float4", "float8":
			if plainNumber.MatchString(*v) {
				return *v
			}
		case "boolean", "bool":
			switch *v {
			case "t", "true":
				return "true"
			case "f", "false":
				return "false"
			}
		}
	}
	return quoteString(*v)
}
