package diff

import "github.com/pgplex/pgschema/ir"

// diffContext provides context about the SQL statement being generated
type diffContext struct {
	Type                DiffType      // e.g., DiffTypeTable, DiffTypeView, DiffTypeFunction
	Operation           DiffOperation // e.g., DiffOperationCreate, DiffOperationAlter, DiffOperationDrop
	Path                string        // e.g., "schema.table" or "schema.table.column"
	Source              DiffSource    // The ddlDiff element that generated this SQL
	CanRunInTransaction bool          // Whether this SQL can run in a transaction
}

// diffCollector collects SQL statements with their context information
type diffCollector struct {
	diffs              []Diff
	pendingForeignKeys []*deferredConstraint
}

// newDiffCollector creates a new diffCollector
func newDiffCollector() *diffCollector {
	return &diffCollector{
		diffs:              []Diff{},
		pendingForeignKeys: nil,
	}
}

// queueDeferredForeignKey schedules an ALTER TABLE ... ADD FOREIGN KEY for a later flush
// (after CREATE and MODIFY phases) so referenced tables and new PK/UNIQUE constraints exist.
func (c *diffCollector) queueDeferredForeignKey(table *ir.Table, constraint *ir.Constraint) {
	if c == nil || table == nil || constraint == nil || constraint.Name == "" {
		return
	}
	c.pendingForeignKeys = append(c.pendingForeignKeys, &deferredConstraint{
		table:      table,
		constraint: constraint,
	})
}

// flushDeferredForeignKeys emits pending foreign keys in dependency order.
func (c *diffCollector) flushDeferredForeignKeys(targetSchema string) {
	if c == nil || len(c.pendingForeignKeys) == 0 {
		return
	}
	sorted := sortDeferredForeignKeys(c.pendingForeignKeys)
	for _, item := range sorted {
		emitDeferredForeignKeyConstraint(item, targetSchema, c)
	}
	c.pendingForeignKeys = nil
}

// collect collects a single SQL statement with its context information
func (c *diffCollector) collect(context *diffContext, stmt string) {
	if context != nil {
		step := Diff{
			Statements: []SQLStatement{{
				SQL:                 stmt,
				CanRunInTransaction: context.CanRunInTransaction,
			}},
			Type:      context.Type,
			Operation: context.Operation,
			Path:      context.Path,
			Source:    context.Source,
		}
		c.diffs = append(c.diffs, step)
	}
}

// collectStatements collects multiple SQL statements as a single Diff
func (c *diffCollector) collectStatements(context *diffContext, statements []SQLStatement) {
	if context != nil && len(statements) > 0 {
		step := Diff{
			Statements: statements,
			Type:       context.Type,
			Operation:  context.Operation,
			Path:       context.Path,
			Source:     context.Source,
		}
		c.diffs = append(c.diffs, step)
	}
}
