package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// stripSchemaPrefix removes the schema prefix from a type name if it matches the target schema.
// It handles both simple type names (e.g., "schema.typename") and type casts within expressions
// (e.g., "'value'::schema.typename" -> "'value'::typename").
func stripSchemaPrefix(typeName, targetSchema string) string {
	if typeName == "" || targetSchema == "" {
		return typeName
	}

	// Check if the type has the target schema prefix at the beginning
	prefix := targetSchema + "."
	if after, found := strings.CutPrefix(typeName, prefix); found {
		return after
	}

	// Also handle type casts within expressions: ::schema.typename -> ::typename
	// This is needed for function parameter default values like 'value'::schema.enum_type
	castPrefix := "::" + targetSchema + "."
	if strings.Contains(typeName, castPrefix) {
		return strings.ReplaceAll(typeName, castPrefix, "::")
	}

	return typeName
}

// stripTempSchemaPrefix removes temporary embedded postgres schema prefixes (pgschema_tmp_*).
// These are used internally during plan generation and should not appear in output DDL.
func stripTempSchemaPrefix(value string) string {
	if value == "" {
		return value
	}

	// Pattern: ::pgschema_tmp_YYYYMMDD_HHMMSS_XXXXXXXX.typename -> ::typename
	// We look for ::pgschema_tmp_ followed by anything until the next dot
	idx := strings.Index(value, "::pgschema_tmp_")
	if idx == -1 {
		return value
	}

	// Find the dot after pgschema_tmp_*
	dotIdx := strings.Index(value[idx+15:], ".")
	if dotIdx == -1 {
		return value
	}

	// Replace ::pgschema_tmp_XXX.typename with ::typename
	prefix := value[idx : idx+15+dotIdx+1] // includes the trailing dot
	return strings.ReplaceAll(value, prefix, "::")
}

// sortConstraintColumnsByPosition sorts constraint columns by their position
func sortConstraintColumnsByPosition(columns []*ir.ConstraintColumn) []*ir.ConstraintColumn {
	sorted := make([]*ir.ConstraintColumn, len(columns))
	copy(sorted, columns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Position < sorted[j].Position
	})
	return sorted
}

// diffTriggers compares triggers between two tables and populates the diff
func diffTriggers(oldTable, newTable *ir.Table, diff *tableDiff) {
	oldTriggers := make(map[string]*ir.Trigger)
	newTriggers := make(map[string]*ir.Trigger)

	if oldTable.Triggers != nil {
		for name, trigger := range oldTable.Triggers {
			oldTriggers[name] = trigger
		}
	}

	if newTable.Triggers != nil {
		for name, trigger := range newTable.Triggers {
			newTriggers[name] = trigger
		}
	}

	// Find added triggers
	for name, trigger := range newTriggers {
		if _, exists := oldTriggers[name]; !exists {
			diff.AddedTriggers = append(diff.AddedTriggers, trigger)
		}
	}

	// Find dropped triggers
	for name, trigger := range oldTriggers {
		if _, exists := newTriggers[name]; !exists {
			diff.DroppedTriggers = append(diff.DroppedTriggers, trigger)
		}
	}

	// Find modified triggers
	for name, newTrigger := range newTriggers {
		if oldTrigger, exists := oldTriggers[name]; exists {
			if !triggersEqual(oldTrigger, newTrigger) {
				diff.ModifiedTriggers = append(diff.ModifiedTriggers, &triggerDiff{
					Old: oldTrigger,
					New: newTrigger,
				})
			}
		}
	}
}

// diffTables compares two tables and returns the differences
// targetSchema is used to normalize type names before comparison
func diffTables(oldTable, newTable *ir.Table, targetSchema string) *tableDiff {
	diff := &tableDiff{
		Table:               newTable,
		AddedColumns:        []*ir.Column{},
		DroppedColumns:      []*ir.Column{},
		ModifiedColumns:     []*ColumnDiff{},
		AddedConstraints:    []*ir.Constraint{},
		DroppedConstraints:  []*ir.Constraint{},
		ModifiedConstraints: []*ConstraintDiff{},
		AddedIndexes:        []*ir.Index{},
		DroppedIndexes:      []*ir.Index{},
		ModifiedIndexes:     []*IndexDiff{},
		AddedTriggers:       []*ir.Trigger{},
		DroppedTriggers:     []*ir.Trigger{},
		ModifiedTriggers:    []*triggerDiff{},
		AddedPolicies:       []*ir.RLSPolicy{},
		DroppedPolicies:     []*ir.RLSPolicy{},
		ModifiedPolicies:    []*policyDiff{},
		RLSChanges:          []*rlsChange{},
	}

	// Build maps for efficient lookup
	oldColumns := make(map[string]*ir.Column)
	newColumns := make(map[string]*ir.Column)

	for _, column := range oldTable.Columns {
		oldColumns[column.Name] = column
	}

	for _, column := range newTable.Columns {
		newColumns[column.Name] = column
	}

	// Find added columns
	for name, column := range newColumns {
		if _, exists := oldColumns[name]; !exists {
			diff.AddedColumns = append(diff.AddedColumns, column)
		}
	}

	// Find dropped columns
	for name, column := range oldColumns {
		if _, exists := newColumns[name]; !exists {
			diff.DroppedColumns = append(diff.DroppedColumns, column)
		}
	}

	// Find modified columns
	for name, newColumn := range newColumns {
		if oldColumn, exists := oldColumns[name]; exists {
			if !columnsEqual(oldColumn, newColumn, targetSchema) {
				diff.ModifiedColumns = append(diff.ModifiedColumns, &ColumnDiff{
					Old: oldColumn,
					New: newColumn,
				})
			}
		}
	}

	// Compare constraints
	oldConstraints := make(map[string]*ir.Constraint)
	newConstraints := make(map[string]*ir.Constraint)

	if oldTable.Constraints != nil {
		for name, constraint := range oldTable.Constraints {
			oldConstraints[name] = constraint
		}
	}

	if newTable.Constraints != nil {
		for name, constraint := range newTable.Constraints {
			newConstraints[name] = constraint
		}
	}

	// Find added constraints
	for name, constraint := range newConstraints {
		if _, exists := oldConstraints[name]; !exists {
			diff.AddedConstraints = append(diff.AddedConstraints, constraint)
		}
	}

	// Find dropped constraints
	for name, constraint := range oldConstraints {
		if _, exists := newConstraints[name]; !exists {
			diff.DroppedConstraints = append(diff.DroppedConstraints, constraint)
		}
	}

	// Find modified constraints
	for name, newConstraint := range newConstraints {
		if oldConstraint, exists := oldConstraints[name]; exists {
			if !constraintsEqual(oldConstraint, newConstraint) {
				diff.ModifiedConstraints = append(diff.ModifiedConstraints, &ConstraintDiff{
					Old: oldConstraint,
					New: newConstraint,
				})
			}
		}
	}

	// Compare indexes
	oldIndexes := make(map[string]*ir.Index)
	newIndexes := make(map[string]*ir.Index)

	for _, index := range oldTable.Indexes {
		oldIndexes[index.Name] = index
	}

	for _, index := range newTable.Indexes {
		newIndexes[index.Name] = index
	}

	// Find added indexes
	for name, index := range newIndexes {
		if _, exists := oldIndexes[name]; !exists {
			diff.AddedIndexes = append(diff.AddedIndexes, index)
		}
	}

	// Find dropped indexes
	for name, index := range oldIndexes {
		if _, exists := newIndexes[name]; !exists {
			diff.DroppedIndexes = append(diff.DroppedIndexes, index)
		}
	}

	// Find modified indexes (comment changes and structural changes)
	for name, newIndex := range newIndexes {
		if oldIndex, exists := oldIndexes[name]; exists {
			structurallyEqual := indexesStructurallyEqual(oldIndex, newIndex)
			commentChanged := oldIndex.Comment != newIndex.Comment

			// If only comments changed, treat as modification
			if structurallyEqual && commentChanged {
				diff.ModifiedIndexes = append(diff.ModifiedIndexes, &IndexDiff{
					Old: oldIndex,
					New: newIndex,
				})
			} else if !structurallyEqual {
				// If structure changed, treat as drop + add for proper online handling
				diff.DroppedIndexes = append(diff.DroppedIndexes, oldIndex)
				diff.AddedIndexes = append(diff.AddedIndexes, newIndex)
			}
		}
	}

	// Compare triggers
	diffTriggers(oldTable, newTable, diff)

	// Compare policies
	oldPolicies := make(map[string]*ir.RLSPolicy)
	newPolicies := make(map[string]*ir.RLSPolicy)

	if oldTable.Policies != nil {
		for name, policy := range oldTable.Policies {
			oldPolicies[name] = policy
		}
	}

	if newTable.Policies != nil {
		for name, policy := range newTable.Policies {
			newPolicies[name] = policy
		}
	}

	// Find added policies
	for name, policy := range newPolicies {
		if _, exists := oldPolicies[name]; !exists {
			diff.AddedPolicies = append(diff.AddedPolicies, policy)
		}
	}

	// Find dropped policies
	for name, policy := range oldPolicies {
		if _, exists := newPolicies[name]; !exists {
			diff.DroppedPolicies = append(diff.DroppedPolicies, policy)
		}
	}

	// Find modified policies
	for name, newPolicy := range newPolicies {
		if oldPolicy, exists := oldPolicies[name]; exists {
			if !policiesEqual(oldPolicy, newPolicy) {
				diff.ModifiedPolicies = append(diff.ModifiedPolicies, &policyDiff{
					Old: oldPolicy,
					New: newPolicy,
				})
			}
		}
	}

	// Check for RLS enable/disable and force changes
	if oldTable.RLSEnabled != newTable.RLSEnabled || oldTable.RLSForced != newTable.RLSForced {
		change := &rlsChange{
			Table: newTable,
		}
		if oldTable.RLSEnabled != newTable.RLSEnabled {
			change.Enabled = &newTable.RLSEnabled
		}
		// Only track FORCE changes if RLS is not being disabled
		// (disabling RLS implicitly clears FORCE, making NO FORCE redundant)
		if oldTable.RLSForced != newTable.RLSForced && newTable.RLSEnabled {
			change.Forced = &newTable.RLSForced
		}
		diff.RLSChanges = append(diff.RLSChanges, change)
	}

	// Check for table comment changes
	if oldTable.Comment != newTable.Comment {
		diff.CommentChanged = true
		diff.OldComment = oldTable.Comment
		diff.NewComment = newTable.Comment
	}

	// Return nil if no changes
	if len(diff.AddedColumns) == 0 && len(diff.DroppedColumns) == 0 &&
		len(diff.ModifiedColumns) == 0 && len(diff.AddedConstraints) == 0 &&
		len(diff.DroppedConstraints) == 0 && len(diff.ModifiedConstraints) == 0 &&
		len(diff.AddedIndexes) == 0 && len(diff.DroppedIndexes) == 0 &&
		len(diff.ModifiedIndexes) == 0 && len(diff.AddedTriggers) == 0 &&
		len(diff.DroppedTriggers) == 0 && len(diff.ModifiedTriggers) == 0 &&
		len(diff.AddedPolicies) == 0 && len(diff.DroppedPolicies) == 0 &&
		len(diff.ModifiedPolicies) == 0 && len(diff.RLSChanges) == 0 &&
		!diff.CommentChanged {
		return nil
	}

	return diff
}

// diffExternalTable compares two external tables and returns only trigger differences
// External tables are not managed by pgschema, so we only track triggers on them
func diffExternalTable(oldTable, newTable *ir.Table) *tableDiff {
	diff := &tableDiff{
		Table:               newTable,
		AddedColumns:        []*ir.Column{},
		DroppedColumns:      []*ir.Column{},
		ModifiedColumns:     []*ColumnDiff{},
		AddedConstraints:    []*ir.Constraint{},
		DroppedConstraints:  []*ir.Constraint{},
		ModifiedConstraints: []*ConstraintDiff{},
		AddedIndexes:        []*ir.Index{},
		DroppedIndexes:      []*ir.Index{},
		ModifiedIndexes:     []*IndexDiff{},
		AddedTriggers:       []*ir.Trigger{},
		DroppedTriggers:     []*ir.Trigger{},
		ModifiedTriggers:    []*triggerDiff{},
		AddedPolicies:       []*ir.RLSPolicy{},
		DroppedPolicies:     []*ir.RLSPolicy{},
		ModifiedPolicies:    []*policyDiff{},
		RLSChanges:          []*rlsChange{},
	}

	// For external tables, only compare triggers (not table structure)
	diffTriggers(oldTable, newTable, diff)

	// Return nil if no trigger changes
	if len(diff.AddedTriggers) == 0 && len(diff.DroppedTriggers) == 0 && len(diff.ModifiedTriggers) == 0 {
		return nil
	}

	return diff
}

type deferredConstraint struct {
	table      *ir.Table
	constraint *ir.Constraint
}

// generateCreateTablesSQL generates CREATE TABLE statements with co-located indexes, policies, and RLS.
// Policies that reference other new tables in the same migration or newly added functions
// (via USING/WITH CHECK expressions) are deferred for creation after all tables and functions
// exist (#373). All other policies are emitted inline.
// It returns deferred policies and foreign key constraints that should be applied after dependent objects exist.
// Tables are assumed to be pre-sorted in topological order for dependency-aware creation.
func generateCreateTablesSQL(
	tables []*ir.Table,
	targetSchema string,
	collector *diffCollector,
	existingTables map[string]bool,
	shouldDeferPolicy func(*ir.RLSPolicy) bool,
) ([]*ir.RLSPolicy, []*deferredConstraint) {
	var deferredPolicies []*ir.RLSPolicy
	var deferredConstraints []*deferredConstraint
	createdTables := make(map[string]bool, len(tables))

	// Process tables in the provided order (already topologically sorted)
	for _, table := range tables {
		// Create the table, deferring FK constraints that reference not-yet-created tables
		sql, tableDeferred := generateTableSQL(table, targetSchema, createdTables, existingTables)
		deferredConstraints = append(deferredConstraints, tableDeferred...)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeTable,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", table.Schema, table.Name),
			Source:              table,
			CanRunInTransaction: true, // CREATE TABLE can run in a transaction
		}

		collector.collect(context, sql)

		// Add table comment
		if table.Comment != "" {
			tableName := qualifyEntityName(table.Schema, table.Name, targetSchema)
			sql := fmt.Sprintf("COMMENT ON TABLE %s IS %s;", tableName, quoteString(table.Comment))

			// Create context for this statement
			context := &diffContext{
				Type:                DiffTypeTableComment,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s", table.Schema, table.Name),
				Source:              table,
				CanRunInTransaction: true,
			}

			collector.collect(context, sql)
		}

		// Add column comments
		for _, column := range table.Columns {
			if column.Comment != "" {
				tableName := qualifyEntityName(table.Schema, table.Name, targetSchema)
				sql := fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s;", tableName, ir.QuoteIdentifier(column.Name), quoteString(column.Comment))

				// Create context for this statement
				context := &diffContext{
					Type:                DiffTypeTableColumnComment,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s.%s", table.Schema, table.Name, column.Name),
					Source:              table,
					CanRunInTransaction: true,
				}

				collector.collect(context, sql)
			}
		}

		// Convert map to slice for indexes
		indexes := make([]*ir.Index, 0, len(table.Indexes))
		for _, index := range table.Indexes {
			indexes = append(indexes, index)
		}
		generateCreateIndexesSQL(indexes, targetSchema, collector)

		// Handle RLS enable/force changes (before creating policies) - only for diff scenarios
		if table.RLSEnabled || table.RLSForced {
			change := &rlsChange{Table: table}
			if table.RLSEnabled {
				enabled := true
				change.Enabled = &enabled
			}
			if table.RLSForced {
				forced := true
				change.Forced = &forced
			}
			rlsChanges := []*rlsChange{change}
			generateRLSChangesSQL(rlsChanges, targetSchema, collector)
		}

		// Collect policies: defer those that reference other new tables or new functions (#373),
		// emit the rest inline with their parent table.
		if len(table.Policies) > 0 {
			var inlinePolicies []*ir.RLSPolicy
			policyNames := sortedKeys(table.Policies)
			for _, name := range policyNames {
				policy := table.Policies[name]
				if shouldDeferPolicy != nil && shouldDeferPolicy(policy) {
					deferredPolicies = append(deferredPolicies, policy)
				} else {
					inlinePolicies = append(inlinePolicies, policy)
				}
			}

			if len(inlinePolicies) > 0 {
				generateCreatePoliciesSQL(inlinePolicies, targetSchema, collector)
			}
		}

		createdTables[fmt.Sprintf("%s.%s", table.Schema, table.Name)] = true
	}

	return deferredPolicies, deferredConstraints
}

func generateDeferredConstraintsSQL(deferred []*deferredConstraint, targetSchema string, collector *diffCollector) {
	for _, item := range deferred {
		constraint := item.constraint
		if constraint == nil || item.table == nil || constraint.Name == "" {
			continue
		}

		columns := sortConstraintColumnsByPosition(constraint.Columns)
		var columnNames []string
		for _, col := range columns {
			columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
		}
		if constraint.IsTemporal && len(columnNames) > 0 {
			columnNames[len(columnNames)-1] = "PERIOD " + columnNames[len(columnNames)-1]
		}

		tableName := getTableNameWithSchema(item.table.Schema, item.table.Name, targetSchema)
		sql := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s FOREIGN KEY (%s) %s;",
			tableName,
			ir.QuoteIdentifier(constraint.Name),
			strings.Join(columnNames, ", "),
			generateForeignKeyClause(constraint, targetSchema, false),
		)

		context := &diffContext{
			Type:                DiffTypeTableConstraint,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", item.table.Schema, item.table.Name, constraint.Name),
			Source:              constraint,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}
}

// generateModifyTablesSQL generates ALTER TABLE statements
func generateModifyTablesSQL(diffs []*tableDiff, droppedTables []*ir.Table, targetSchema string, collector *diffCollector) {
	// Build a set of tables being dropped (CASCADE will remove their dependent FK constraints)
	droppedTableSet := make(map[string]bool, len(droppedTables))
	for _, t := range droppedTables {
		droppedTableSet[t.Schema+"."+t.Name] = true
	}

	// Diffs are already sorted by the Diff operation
	for _, diff := range diffs {
		// Pass collector to generateAlterTableStatements to collect with proper context
		diff.generateAlterTableStatements(targetSchema, collector, droppedTableSet)
	}
}

// generateDropTablesSQL generates DROP TABLE statements
// Tables are assumed to be pre-sorted in reverse topological order for dependency-aware dropping
func generateDropTablesSQL(tables []*ir.Table, targetSchema string, collector *diffCollector) {
	// Process tables in the provided order (already reverse topologically sorted)
	for _, table := range tables {
		tableName := qualifyEntityName(table.Schema, table.Name, targetSchema)
		sql := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", tableName)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeTable,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", table.Schema, table.Name),
			Source:              table,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateTableSQL generates CREATE TABLE statement and returns any deferred FK constraints
func generateTableSQL(table *ir.Table, targetSchema string, createdTables map[string]bool, existingTables map[string]bool) (string, []*deferredConstraint) {
	// Only include table name without schema if it's in the target schema
	tableName := ir.QualifyEntityNameWithQuotes(table.Schema, table.Name, targetSchema)

	var parts []string
	parts = append(parts, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (", tableName))

	// Add columns
	var columnParts []string
	for _, column := range table.Columns {
		// Build column definition with SERIAL detection
		var builder strings.Builder
		writeColumnDefinitionToBuilder(&builder, table, column, targetSchema)
		columnParts = append(columnParts, fmt.Sprintf("    %s", builder.String()))
	}

	// Add LIKE clauses
	for _, likeClause := range table.LikeClauses {
		likeTableName := ir.QualifyEntityNameWithQuotes(likeClause.SourceSchema, likeClause.SourceTable, targetSchema)
		likeSQL := fmt.Sprintf("LIKE %s", likeTableName)
		if likeClause.Options != "" {
			likeSQL += " " + likeClause.Options
		}
		columnParts = append(columnParts, fmt.Sprintf("    %s", likeSQL))
	}

	// Add constraints inline in the correct order (PRIMARY KEY, UNIQUE, FOREIGN KEY)
	inlineConstraints := getInlineConstraintsForTable(table)
	var deferred []*deferredConstraint
	currentKey := fmt.Sprintf("%s.%s", table.Schema, table.Name)
	for _, constraint := range inlineConstraints {
		if shouldDeferConstraint(table, constraint, currentKey, createdTables, existingTables) {
			deferred = append(deferred, &deferredConstraint{
				table:      table,
				constraint: constraint,
			})
			continue
		}
		constraintDef := generateConstraintSQL(constraint, targetSchema)
		if constraintDef != "" {
			columnParts = append(columnParts, fmt.Sprintf("    %s", constraintDef))
		}
	}

	parts = append(parts, strings.Join(columnParts, ",\n"))

	// Add partition clause for partitioned tables
	if table.IsPartitioned && table.PartitionStrategy != "" && table.PartitionKey != "" {
		parts = append(parts, fmt.Sprintf(") PARTITION BY %s (%s);", table.PartitionStrategy, table.PartitionKey))
	} else {
		parts = append(parts, ");")
	}

	return strings.Join(parts, "\n"), deferred
}

func shouldDeferConstraint(table *ir.Table, constraint *ir.Constraint, currentKey string, createdTables map[string]bool, existingTables map[string]bool) bool {
	if constraint == nil || constraint.Type != ir.ConstraintTypeForeignKey {
		return false
	}

	refSchema := constraint.ReferencedSchema
	if refSchema == "" {
		refSchema = table.Schema
	}
	if constraint.ReferencedTable == "" {
		return false
	}
	refKey := fmt.Sprintf("%s.%s", refSchema, constraint.ReferencedTable)
	if refKey == currentKey {
		return false
	}

	// Check if the referenced table exists (either being created or already exists)
	if existingTables != nil && existingTables[refKey] {
		return false // Table exists, no need to defer
	}
	if createdTables != nil && createdTables[refKey] {
		return false // Table already created in this operation
	}

	// Referenced table doesn't exist yet, defer the constraint
	return true
}

// generateAlterTableStatements generates SQL statements for table modifications
// Note: DroppedTriggers are skipped here because they are already processed in the DROP phase
// (see generateDropTriggersFromModifiedTables in trigger.go)
// droppedTableSet contains "schema.table" keys for tables being dropped with CASCADE;
// FK constraints referencing these tables are skipped since CASCADE already removes them.
func (td *tableDiff) generateAlterTableStatements(targetSchema string, collector *diffCollector, droppedTableSet map[string]bool) {
	// Drop constraints first (before dropping columns) - already sorted by the Diff operation
	for _, constraint := range td.DroppedConstraints {
		// Skip FK constraints whose referenced table is being dropped with CASCADE,
		// since the CASCADE will already remove the constraint. (#382)
		if constraint.Type == ir.ConstraintTypeForeignKey && constraint.ReferencedTable != "" {
			refKey := constraint.ReferencedSchema + "." + constraint.ReferencedTable
			if droppedTableSet[refKey] {
				continue
			}
		}

		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		sql := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", tableName, ir.QuoteIdentifier(constraint.Name))

		context := &diffContext{
			Type:                DiffTypeTableConstraint,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
			Source:              constraint,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Drop columns - already sorted by the Diff operation
	for _, column := range td.DroppedColumns {
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		sql := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tableName, ir.QuoteIdentifier(column.Name))

		context := &diffContext{
			Type:                DiffTypeTableColumn,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, column.Name),
			Source:              column,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Track constraints that are added inline with columns to avoid duplicate generation
	inlineConstraints := make(map[string]bool)

	// Add new columns - already sorted by the Diff operation
	for _, column := range td.AddedColumns {
		// Check if column is part of any primary key constraint for NOT NULL handling
		isPartOfAnyPK := false
		for _, constraint := range td.AddedConstraints {
			if constraint.Type == ir.ConstraintTypePrimaryKey {
				for _, col := range constraint.Columns {
					if col.Name == column.Name {
						isPartOfAnyPK = true
						break
					}
				}
				if isPartOfAnyPK {
					break
				}
			}
		}

		// Build column type and strip schema prefix if it matches target schema
		columnType := formatColumnDataType(column)
		columnType = stripSchemaPrefix(columnType, targetSchema)
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)

		// Build and append all column clauses
		clauses := buildColumnClauses(column, isPartOfAnyPK, td.Table.Schema, targetSchema)

		// Check for single-column constraints that can be added inline
		var inlineConstraint string
		for _, constraint := range td.AddedConstraints {
			// Only add inline for single-column constraints
			if len(constraint.Columns) == 1 && constraint.Columns[0].Name == column.Name {
				switch constraint.Type {
				case ir.ConstraintTypePrimaryKey:
					inlineConstraint = fmt.Sprintf(" CONSTRAINT %s PRIMARY KEY", ir.QuoteIdentifier(constraint.Name))
				case ir.ConstraintTypeUnique:
					inlineConstraint = fmt.Sprintf(" CONSTRAINT %s UNIQUE", ir.QuoteIdentifier(constraint.Name))
				case ir.ConstraintTypeForeignKey:
					// For FK, use the generateForeignKeyClause with inline=true
					fkClause := generateForeignKeyClause(constraint, targetSchema, true)
					inlineConstraint = fmt.Sprintf(" CONSTRAINT %s%s", ir.QuoteIdentifier(constraint.Name), fkClause)
				case ir.ConstraintTypeCheck:
					// For CHECK, format the clause inline
					checkExpr := constraint.CheckClause
					// Strip "CHECK" prefix if present
					if len(checkExpr) >= 5 && strings.EqualFold(checkExpr[:5], "check") {
						checkExpr = strings.TrimSpace(checkExpr[5:])
					}
					checkExpr = strings.TrimSpace(checkExpr)
					// Ensure parentheses
					if !strings.HasPrefix(checkExpr, "(") {
						checkExpr = "(" + checkExpr + ")"
					}
					inlineConstraint = fmt.Sprintf(" CONSTRAINT %s CHECK %s", ir.QuoteIdentifier(constraint.Name), checkExpr)
				}

				if inlineConstraint != "" {
					inlineConstraints[constraint.Name] = true
					break
				}
			}
		}

		// Build base ALTER TABLE ADD COLUMN statement
		// Use newline format if there's an inline constraint for better readability
		var stmt string
		if inlineConstraint != "" {
			stmt = fmt.Sprintf("ALTER TABLE %s\nADD COLUMN %s %s%s%s",
				tableName, ir.QuoteIdentifier(column.Name), columnType, clauses, inlineConstraint)
		} else {
			stmt = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s%s",
				tableName, ir.QuoteIdentifier(column.Name), columnType, clauses)
		}

		context := &diffContext{
			Type:                DiffTypeTableColumn,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, column.Name),
			Source:              column,
			CanRunInTransaction: true,
		}
		collector.collect(context, stmt+";")
	}

	// Add comments for new columns
	for _, column := range td.AddedColumns {
		if column.Comment != "" {
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			sql := fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s;", tableName, ir.QuoteIdentifier(column.Name), quoteString(column.Comment))

			context := &diffContext{
				Type:                DiffTypeTableColumnComment,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, column.Name),
				Source:              column,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Modify existing columns - already sorted by the Diff operation
	for _, ColumnDiff := range td.ModifiedColumns {
		// Generate column modification statements and collect as a single step
		columnStatements := ColumnDiff.generateColumnSQL(td.Table.Schema, td.Table.Name, targetSchema)
		// Emit separate diffs for each column operation
		for _, stmt := range columnStatements {
			context := &diffContext{
				Type:                DiffTypeTableColumn,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, ColumnDiff.New.Name),
				Source:              ColumnDiff,
				CanRunInTransaction: true,
			}

			collector.collect(context, stmt)
		}
	}

	// Add new constraints - already sorted by the Diff operation
	for _, constraint := range td.AddedConstraints {
		// Skip constraints that were already added inline with columns
		if inlineConstraints[constraint.Name] {
			continue
		}

		switch constraint.Type {
		case ir.ConstraintTypeUnique:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = columnNames[len(columnNames)-1] + " WITHOUT OVERLAPS"
			}
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			sql := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s UNIQUE (%s);",
				tableName, ir.QuoteIdentifier(constraint.Name), strings.Join(columnNames, ", "))

			context := &diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)

		case ir.ConstraintTypeCheck:
			// Ensure CHECK clause has outer parentheses around the full expression
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			clause := ensureCheckClauseParens(constraint.CheckClause)
			canonicalSQL := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s %s;",
				tableName, ir.QuoteIdentifier(constraint.Name), clause)

			context := &diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}
			collector.collect(context, canonicalSQL)

		case ir.ConstraintTypeForeignKey:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = "PERIOD " + columnNames[len(columnNames)-1]
			}

			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			canonicalSQL := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s FOREIGN KEY (%s) %s;",
				tableName, ir.QuoteIdentifier(constraint.Name),
				strings.Join(columnNames, ", "),
				generateForeignKeyClause(constraint, targetSchema, false))

			context := &diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}
			collector.collect(context, canonicalSQL)

		case ir.ConstraintTypePrimaryKey:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = columnNames[len(columnNames)-1] + " WITHOUT OVERLAPS"
			}
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			sql := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s PRIMARY KEY (%s);",
				tableName, ir.QuoteIdentifier(constraint.Name), strings.Join(columnNames, ", "))

			context := &diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)

		case ir.ConstraintTypeExclusion:
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			sql := fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s %s;",
				tableName, ir.QuoteIdentifier(constraint.Name), constraint.ExclusionDefinition)

			context := &diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Handle modified constraints - drop and recreate them as separate operations
	for _, ConstraintDiff := range td.ModifiedConstraints {
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		constraint := ConstraintDiff.New

		// Step 1: Drop the old constraint
		dropSQL := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", tableName, ir.QuoteIdentifier(ConstraintDiff.Old.Name))
		dropContext := &diffContext{
			Type:                DiffTypeTableConstraint,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, ConstraintDiff.Old.Name),
			Source:              ConstraintDiff.Old,
			CanRunInTransaction: true,
		}
		collector.collect(dropContext, dropSQL)

		// Step 2: Add new constraint
		var addSQL string
		switch constraint.Type {
		case ir.ConstraintTypeUnique:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = columnNames[len(columnNames)-1] + " WITHOUT OVERLAPS"
			}
			addSQL = fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s UNIQUE (%s);",
				tableName, ir.QuoteIdentifier(constraint.Name), strings.Join(columnNames, ", "))

		case ir.ConstraintTypeCheck:
			// Add CHECK constraint with ensured outer parentheses
			addSQL = fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s %s;",
				tableName, ir.QuoteIdentifier(constraint.Name), ensureCheckClauseParens(constraint.CheckClause))

		case ir.ConstraintTypeForeignKey:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = "PERIOD " + columnNames[len(columnNames)-1]
			}

			addSQL = fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s FOREIGN KEY (%s) %s;",
				tableName, ir.QuoteIdentifier(constraint.Name),
				strings.Join(columnNames, ", "),
				generateForeignKeyClause(constraint, targetSchema, false))

		case ir.ConstraintTypePrimaryKey:
			// Sort columns by position
			columns := sortConstraintColumnsByPosition(constraint.Columns)
			var columnNames []string
			for _, col := range columns {
				columnNames = append(columnNames, ir.QuoteIdentifier(col.Name))
			}
			if constraint.IsTemporal && len(columnNames) > 0 {
				columnNames[len(columnNames)-1] = columnNames[len(columnNames)-1] + " WITHOUT OVERLAPS"
			}
			addSQL = fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s PRIMARY KEY (%s);",
				tableName, ir.QuoteIdentifier(constraint.Name), strings.Join(columnNames, ", "))

		case ir.ConstraintTypeExclusion:
			addSQL = fmt.Sprintf("ALTER TABLE %s\nADD CONSTRAINT %s %s;",
				tableName, ir.QuoteIdentifier(constraint.Name), constraint.ExclusionDefinition)
		}

		addContext := &diffContext{
			Type:                DiffTypeTableConstraint,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
			Source:              constraint,
			CanRunInTransaction: true,
		}

		collector.collect(addContext, addSQL)
	}

	// Handle RLS changes
	for _, rlsChange := range td.RLSChanges {
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)

		// Handle ENABLE/DISABLE changes
		if rlsChange.Enabled != nil {
			var sql string
			var operation DiffOperation
			if *rlsChange.Enabled {
				sql = fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", tableName)
				operation = DiffOperationCreate
			} else {
				sql = fmt.Sprintf("ALTER TABLE %s DISABLE ROW LEVEL SECURITY;", tableName)
				operation = DiffOperationDrop
			}

			context := &diffContext{
				Type:                DiffTypeTableRLS,
				Operation:           operation,
				Path:                fmt.Sprintf("%s.%s", td.Table.Schema, td.Table.Name),
				Source:              rlsChange,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}

		// Handle FORCE/NO FORCE changes
		if rlsChange.Forced != nil {
			var sql string
			var operation DiffOperation
			if *rlsChange.Forced {
				sql = fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY;", tableName)
				operation = DiffOperationAlter
			} else {
				sql = fmt.Sprintf("ALTER TABLE %s NO FORCE ROW LEVEL SECURITY;", tableName)
				operation = DiffOperationAlter
			}

			context := &diffContext{
				Type:                DiffTypeTableRLS,
				Operation:           operation,
				Path:                fmt.Sprintf("%s.%s", td.Table.Schema, td.Table.Name),
				Source:              rlsChange,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Drop policies - already sorted by the Diff operation
	for _, policy := range td.DroppedPolicies {
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		sql := fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;", ir.QuoteIdentifier(policy.Name), tableName)

		context := &diffContext{
			Type:                DiffTypeTablePolicy,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, policy.Name),
			Source:              policy,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Drop triggers - skipped here because they are already dropped in the DROP phase
	// (see generateDropTriggersFromModifiedTables in trigger.go)

	// Add triggers - already sorted by the Diff operation
	for _, trigger := range td.AddedTriggers {
		sql := generateTriggerSQLWithMode(trigger, targetSchema)

		context := &diffContext{
			Type:                DiffTypeTableTrigger,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, trigger.Name),
			Source:              trigger,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Add policies - already sorted by the Diff operation
	for _, policy := range td.AddedPolicies {
		sql := generatePolicySQL(policy, targetSchema)

		context := &diffContext{
			Type:                DiffTypeTablePolicy,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, policy.Name),
			Source:              policy,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Modify triggers - already sorted by the Diff operation
	for _, triggerDiff := range td.ModifiedTriggers {
		// Constraint triggers don't support CREATE OR REPLACE, so we need to DROP and CREATE
		if triggerDiff.New.IsConstraint {
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)

			// Step 1: DROP the old trigger
			dropSQL := fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s;", triggerDiff.Old.Name, tableName)
			dropContext := &diffContext{
				Type:                DiffTypeTableTrigger,
				Operation:           DiffOperationDrop,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, triggerDiff.Old.Name),
				Source:              triggerDiff.Old,
				CanRunInTransaction: true,
			}
			collector.collect(dropContext, dropSQL)

			// Step 2: CREATE the new constraint trigger
			createSQL := generateTriggerSQLWithMode(triggerDiff.New, targetSchema)
			createContext := &diffContext{
				Type:                DiffTypeTableTrigger,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, triggerDiff.New.Name),
				Source:              triggerDiff.New,
				CanRunInTransaction: true,
			}
			collector.collect(createContext, createSQL)
		} else {
			// Use CREATE OR REPLACE for regular triggers
			sql := generateTriggerSQLWithMode(triggerDiff.New, targetSchema)

			context := &diffContext{
				Type:                DiffTypeTableTrigger,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, triggerDiff.New.Name),
				Source:              triggerDiff,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Modify policies - already sorted by the Diff operation
	for _, policyDiff := range td.ModifiedPolicies {
		// Check if this policy needs to be recreated (DROP + CREATE)
		if needsRecreate(policyDiff.Old, policyDiff.New) {
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			// Drop and recreate policy for modification
			sql := fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;", ir.QuoteIdentifier(policyDiff.Old.Name), tableName)

			context := &diffContext{
				Type:                DiffTypeTablePolicy,
				Operation:           DiffOperationDrop,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, policyDiff.Old.Name),
				Source:              policyDiff,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)

			sql = generatePolicySQL(policyDiff.New, targetSchema)
			context = &diffContext{
				Type:                DiffTypeTablePolicy,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, policyDiff.New.Name),
				Source:              policyDiff,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		} else {
			// Use ALTER POLICY for simple changes
			sql := generateAlterPolicySQL(policyDiff.Old, policyDiff.New, targetSchema)

			context := &diffContext{
				Type:                DiffTypeTablePolicy,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, policyDiff.New.Name),
				Source:              policyDiff,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Handle table comment changes
	if td.CommentChanged {
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		var sql string
		if td.NewComment == "" {
			sql = fmt.Sprintf("COMMENT ON TABLE %s IS NULL;", tableName)
		} else {
			sql = fmt.Sprintf("COMMENT ON TABLE %s IS %s;", tableName, quoteString(td.NewComment))
		}

		context := &diffContext{
			Type:                DiffTypeTableComment,
			Operation:           DiffOperationAlter,
			Path:                fmt.Sprintf("%s.%s", td.Table.Schema, td.Table.Name),
			Source:              td,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Handle column comment changes
	for _, colDiff := range td.ModifiedColumns {
		if colDiff.Old.Comment != colDiff.New.Comment {
			tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
			var sql string
			if colDiff.New.Comment == "" {
				sql = fmt.Sprintf("COMMENT ON COLUMN %s.%s IS NULL;", tableName, ir.QuoteIdentifier(colDiff.New.Name))
			} else {
				sql = fmt.Sprintf("COMMENT ON COLUMN %s.%s IS %s;", tableName, ir.QuoteIdentifier(colDiff.New.Name), quoteString(colDiff.New.Comment))
			}

			context := &diffContext{
				Type:                DiffTypeTableColumnComment,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, colDiff.New.Name),
				Source:              colDiff,
				CanRunInTransaction: true,
			}
			collector.collect(context, sql)
		}
	}

	// Handle index modifications using shared function
	generateIndexModifications(
		td.DroppedIndexes,
		td.AddedIndexes,
		td.ModifiedIndexes,
		targetSchema,
		DiffTypeTableIndex,
		DiffTypeTableIndexComment,
		collector,
	)
}

// ensureCheckClauseParens guarantees that a CHECK clause string contains
// exactly one pair of outer parentheses around the full boolean expression.
// It expects input in the form: "CHECK <expr>" or "CHECK(<expr>)" or "CHECK (<expr>)".
func ensureCheckClauseParens(s string) string {
	t := strings.TrimSpace(s)
	// Normalize leading "CHECK" token
	if len(t) >= 5 && strings.EqualFold(t[:5], "check") {
		t = t[5:]
	}
	expr := strings.TrimSpace(t)

	// Check if expression is already properly wrapped in parentheses
	// by counting parenthesis depth to ensure the outer pair wraps the full expression
	if len(expr) >= 2 && expr[0] == '(' {
		depth := 0
		for i := 0; i < len(expr); i++ {
			switch expr[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					if i == len(expr)-1 {
						// The outermost paren pair wraps the full expression
						return "CHECK " + expr
					}
					// Leading '(' closes before the end -> not fully wrapped
					break
				}
			}
		}
	}

	return "CHECK (" + expr + ")"
}

// writeColumnDefinitionToBuilder builds column definitions with SERIAL detection and proper formatting
// This is moved from ir/table.go to consolidate SQL generation in the diff module
func writeColumnDefinitionToBuilder(builder *strings.Builder, table *ir.Table, column *ir.Column, targetSchema string) {
	builder.WriteString(ir.QuoteIdentifier(column.Name))
	builder.WriteString(" ")

	// Data type - handle array types and precision/scale for appropriate types
	dataType := formatColumnDataTypeForCreate(column)

	// Strip schema prefix if it matches the target schema
	dataType = stripSchemaPrefix(dataType, targetSchema)

	builder.WriteString(dataType)

	// Check if column is part of any primary key constraint for NOT NULL handling
	isPartOfAnyPK := false
	for _, constraint := range table.Constraints {
		if constraint.Type == ir.ConstraintTypePrimaryKey {
			for _, col := range constraint.Columns {
				if col.Name == column.Name {
					isPartOfAnyPK = true
					break
				}
			}
			if isPartOfAnyPK {
				break
			}
		}
	}

	// Build and append all column clauses
	clauses := buildColumnClauses(column, isPartOfAnyPK, table.Schema, targetSchema)
	builder.WriteString(clauses)
}

// buildColumnClauses builds the SQL clauses for a column definition (works for both CREATE TABLE and ALTER TABLE)
// Returns the clauses as a string to be appended to the column name and type
// Order follows PostgreSQL documentation: https://www.postgresql.org/docs/current/sql-altertable.html
func buildColumnClauses(column *ir.Column, isPartOfAnyPK bool, tableSchema string, targetSchema string) string {
	var parts []string

	// 1. Identity columns (must come early, before DEFAULT)
	if column.Identity != nil {
		switch column.Identity.Generation {
		case "ALWAYS":
			parts = append(parts, "GENERATED ALWAYS AS IDENTITY")
		case "BY DEFAULT":
			parts = append(parts, "GENERATED BY DEFAULT AS IDENTITY")
		}
	}

	// 2. DEFAULT (skip for SERIAL, identity, or generated columns)
	if column.DefaultValue != nil && column.Identity == nil && !column.IsGenerated && !isSerialColumn(column) {
		// DefaultValue is already normalized by ir.normalizeColumn
		// (schema qualifiers and sequence references are handled there)
		parts = append(parts, fmt.Sprintf("DEFAULT %s", *column.DefaultValue))
	}

	// 3. Generated column syntax (must come before constraints)
	if column.IsGenerated && column.GeneratedExpr != nil {
		// TODO: Add support for GENERATED ALWAYS AS (...) VIRTUAL when PostgreSQL 18 is supported
		parts = append(parts, fmt.Sprintf("GENERATED ALWAYS AS (%s) STORED", *column.GeneratedExpr))
	}

	// 4. NOT NULL (skip for PK including multi-column PKs, identity, and SERIAL)
	if !column.IsNullable && column.Identity == nil && !isSerialColumn(column) && !isPartOfAnyPK {
		parts = append(parts, "NOT NULL")
	}

	// Note: No inline constraints (PRIMARY KEY, UNIQUE, FOREIGN KEY) are added here
	// ALL constraints are now handled as table-level constraints for consistency
	// This ensures all constraint names are preserved and provides cleaner formatting

	if len(parts) == 0 {
		return ""
	}

	result := " " + strings.Join(parts, " ")
	return result
}

// isSerialColumn checks if a column is a SERIAL column (integer type with nextval default)
func isSerialColumn(column *ir.Column) bool {
	// Check if column has nextval default
	if column.DefaultValue == nil || !strings.Contains(*column.DefaultValue, "nextval") {
		return false
	}

	// Check if column is an integer type
	switch column.DataType {
	case "integer", "int4", "smallint", "int2", "bigint", "int8":
		return true
	default:
		return false
	}
}

// formatColumnDataType formats a column's data type with appropriate modifiers for ALTER TABLE statements
func formatColumnDataType(column *ir.Column) string {
	dataType := column.DataType

	// Handle SERIAL types
	if isSerialColumn(column) {
		switch column.DataType {
		case "smallint", "int2":
			return "smallserial"
		case "bigint", "int8":
			return "bigserial"
		default:
			return "serial"
		}
	}

	// Keep terse forms like timestamptz as preferred

	// Add precision/scale/length modifiers
	if column.MaxLength != nil && (dataType == "varchar" || dataType == "character varying") {
		return fmt.Sprintf("varchar(%d)", *column.MaxLength)
	} else if column.MaxLength != nil && dataType == "character" {
		return fmt.Sprintf("character(%d)", *column.MaxLength)
	} else if column.Precision != nil && column.Scale != nil && (dataType == "numeric" || dataType == "decimal") {
		return fmt.Sprintf("%s(%d,%d)", dataType, *column.Precision, *column.Scale)
	} else if column.Precision != nil && (dataType == "numeric" || dataType == "decimal") {
		return fmt.Sprintf("%s(%d)", dataType, *column.Precision)
	}

	return dataType
}

// formatColumnDataTypeForCreate formats a column's data type with appropriate modifiers for CREATE TABLE statements
func formatColumnDataTypeForCreate(column *ir.Column) string {
	dataType := column.DataType

	// Handle SERIAL types (uppercase for CREATE TABLE)
	if isSerialColumn(column) {
		switch column.DataType {
		case "smallint", "int2":
			return "SMALLSERIAL"
		case "bigint", "int8":
			return "BIGSERIAL"
		default:
			return "SERIAL"
		}
	}

	// Keep timestamptz as-is for CREATE TABLE (don't convert to verbose form)

	// Add precision/scale/length modifiers
	if column.MaxLength != nil && (dataType == "varchar" || dataType == "character varying") {
		return fmt.Sprintf("varchar(%d)", *column.MaxLength)
	} else if column.MaxLength != nil && dataType == "character" {
		return fmt.Sprintf("character(%d)", *column.MaxLength)
	} else if column.Precision != nil && column.Scale != nil && (dataType == "numeric" || dataType == "decimal") {
		return fmt.Sprintf("%s(%d,%d)", dataType, *column.Precision, *column.Scale)
	} else if column.Precision != nil && (dataType == "numeric" || dataType == "decimal") {
		return fmt.Sprintf("%s(%d)", dataType, *column.Precision)
	}

	return dataType
}

// indexesStructurallyEqual compares two indexes for structural equality
// excluding comments and other metadata that don't require index recreation
func indexesStructurallyEqual(oldIndex, newIndex *ir.Index) bool {
	// Compare basic properties that would require recreation
	if oldIndex.Type != newIndex.Type ||
		oldIndex.Method != newIndex.Method ||
		oldIndex.IsPartial != newIndex.IsPartial ||
		oldIndex.IsExpression != newIndex.IsExpression ||
		oldIndex.NullsNotDistinct != newIndex.NullsNotDistinct ||
		oldIndex.Where != newIndex.Where {
		return false
	}

	// Compare column count
	if len(oldIndex.Columns) != len(newIndex.Columns) {
		return false
	}

	// Compare each column's properties
	for i, oldCol := range oldIndex.Columns {
		newCol := newIndex.Columns[i]
		if oldCol.Name != newCol.Name ||
			oldCol.Position != newCol.Position ||
			oldCol.Direction != newCol.Direction ||
			oldCol.Operator != newCol.Operator {
			return false
		}
	}

	return true
}

// generateForeignKeyClause generates the REFERENCES clause with all foreign key options
// Works for both inline single-column and multi-column constraint foreign keys
func generateForeignKeyClause(constraint *ir.Constraint, targetSchema string, inline bool) string {
	referencedTableName := getTableNameWithSchema(constraint.ReferencedSchema, constraint.ReferencedTable, targetSchema)

	var clause string
	if inline {
		clause = fmt.Sprintf(" REFERENCES %s", referencedTableName)
	} else {
		clause = fmt.Sprintf("REFERENCES %s", referencedTableName)
	}

	// Add referenced columns - always with space for consistency
	if len(constraint.ReferencedColumns) > 0 {
		if len(constraint.ReferencedColumns) == 1 {
			// Single column
			clause += fmt.Sprintf(" (%s)", constraint.ReferencedColumns[0].Name)
		} else {
			// Multiple columns - sort by position
			refColumns := sortConstraintColumnsByPosition(constraint.ReferencedColumns)
			var refColumnNames []string
			for _, col := range refColumns {
				refColumnNames = append(refColumnNames, col.Name)
			}
			if constraint.IsTemporal && len(refColumnNames) > 0 {
				refColumnNames[len(refColumnNames)-1] = "PERIOD " + refColumnNames[len(refColumnNames)-1]
			}
			clause += fmt.Sprintf(" (%s)", strings.Join(refColumnNames, ", "))
		}
	}

	// Add referential actions
	if constraint.UpdateRule != "" && constraint.UpdateRule != "NO ACTION" {
		clause += fmt.Sprintf(" ON UPDATE %s", constraint.UpdateRule)
	}
	if constraint.DeleteRule != "" && constraint.DeleteRule != "NO ACTION" {
		clause += fmt.Sprintf(" ON DELETE %s", constraint.DeleteRule)
	}

	// Add deferrable clause
	if constraint.Deferrable {
		if constraint.InitiallyDeferred {
			clause += " DEFERRABLE INITIALLY DEFERRED"
		} else {
			clause += " DEFERRABLE"
		}
	}

	return clause
}
