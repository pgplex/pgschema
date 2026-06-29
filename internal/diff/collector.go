package diff

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
	diffs []Diff

	// qualifySchema forces emitted DDL to always schema-qualify entity names, even
	// for the target schema (set by `dump --qualify-schema`). Default false preserves
	// the standard "smart qualification" (omit the target-schema prefix).
	qualifySchema bool
}

// newDiffCollector creates a new diffCollector
func newDiffCollector() *diffCollector {
	return &diffCollector{
		diffs: []Diff{},
	}
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
