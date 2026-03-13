package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateIndexesSQL generates CREATE INDEX statements for table indexes
func generateCreateIndexesSQL(indexes []*ir.Index, targetSchema string, collector *diffCollector) {
	generateCreateIndexesSQLWithType(indexes, targetSchema, collector, DiffTypeTableIndex, DiffTypeTableIndexComment)
}

// generateCreateIndexesSQLWithType generates CREATE INDEX statements with specified diff types
func generateCreateIndexesSQLWithType(indexes []*ir.Index, targetSchema string, collector *diffCollector, indexType DiffType, commentType DiffType) {
	// Sort indexes by name for consistent ordering
	sortedIndexes := make([]*ir.Index, len(indexes))
	copy(sortedIndexes, indexes)
	sort.Slice(sortedIndexes, func(i, j int) bool {
		return sortedIndexes[i].Name < sortedIndexes[j].Name
	})

	for _, index := range sortedIndexes {
		// Skip primary key indexes as they're handled with constraints
		if index.Type == ir.IndexTypePrimary {
			continue
		}

		canonicalSQL := generateIndexSQL(index, targetSchema, false) // Always generate canonical form

		// Create context for this statement
		context := &diffContext{
			Type:                indexType,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
			Source:              index,
			CanRunInTransaction: true,
		}

		collector.collect(context, canonicalSQL)

		// Add index comment
		if index.Comment != "" {
			indexName := qualifyEntityName(index.Schema, index.Name, targetSchema)
			sql := fmt.Sprintf("COMMENT ON INDEX %s IS %s;", indexName, quoteString(index.Comment))

			// Create context for this statement
			context := &diffContext{
				Type:                commentType,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
				Source:              index,
				CanRunInTransaction: true, // Comments can always run in a transaction
			}

			collector.collect(context, sql)
		}
	}
}

// generateIndexSQL generates CREATE INDEX statement
func generateIndexSQL(index *ir.Index, targetSchema string, isConcurrent bool) string {
	return generateIndexSQLWithName(index, index.Name, targetSchema, isConcurrent)
}

// generateIndexSQLWithName generates CREATE INDEX statement with custom name
func generateIndexSQLWithName(index *ir.Index, indexName string, targetSchema string, isConcurrent bool) string {
	var builder strings.Builder

	// CREATE [UNIQUE] INDEX [CONCURRENTLY] IF NOT EXISTS
	builder.WriteString("CREATE ")
	if index.Type == ir.IndexTypeUnique {
		builder.WriteString("UNIQUE ")
	}
	builder.WriteString("INDEX ")
	if isConcurrent {
		builder.WriteString("CONCURRENTLY ")
	}
	builder.WriteString("IF NOT EXISTS ")

	// Index name
	builder.WriteString(ir.QuoteIdentifier(indexName))
	builder.WriteString(" ON ")

	// Table name with proper schema qualification
	tableName := getTableNameWithSchema(index.Schema, index.Table, targetSchema)
	builder.WriteString(tableName)

	// Index method - only include if not btree (the default)
	if index.Method != "" && index.Method != "btree" {
		builder.WriteString(" USING ")
		builder.WriteString(index.Method)
	}

	// Columns
	builder.WriteString(" (")
	for i, col := range index.Columns {
		if i > 0 {
			builder.WriteString(", ")
		}

		// Use column name as-is from pg_get_indexdef()
		// pg_get_indexdef() already handles parenthesization correctly:
		// - Regular columns: column_name
		// - Expressions: ((expression))
		builder.WriteString(col.Name)

		// Add operator class if specified (non-default operator class)
		if col.Operator != "" {
			builder.WriteString(" ")
			builder.WriteString(col.Operator)
		}

		// Add direction if specified
		if col.Direction != "" && col.Direction != "ASC" {
			builder.WriteString(" ")
			builder.WriteString(col.Direction)
		}
	}
	builder.WriteString(")")

	// NULLS NOT DISTINCT for unique indexes (PostgreSQL 15+)
	if index.NullsNotDistinct {
		builder.WriteString(" NULLS NOT DISTINCT")
	}

	// WHERE clause for partial indexes
	if index.IsPartial && index.Where != "" {
		builder.WriteString(" WHERE ")
		builder.WriteString(index.Where)
	}

	// Add semicolon at the end
	builder.WriteString(";")

	return builder.String()
}

// generateIndexModifications handles index drops, adds, and online replacements
// Works for both table indexes and materialized view indexes
func generateIndexModifications(
	droppedIndexes []*ir.Index,
	addedIndexes []*ir.Index,
	modifiedIndexes []*IndexDiff,
	targetSchema string,
	indexDiffType DiffType,
	commentDiffType DiffType,
	collector *diffCollector,
) {
	// Identify indexes that need online replacement (dropped and added with same name)
	onlineReplacements := make(map[string]*ir.Index)
	regularDrops := []*ir.Index{}

	for _, droppedIndex := range droppedIndexes {
		foundReplacement := false
		for _, addedIndex := range addedIndexes {
			if droppedIndex.Name == addedIndex.Name {
				onlineReplacements[droppedIndex.Name] = addedIndex
				foundReplacement = true
				break
			}
		}
		if !foundReplacement {
			regularDrops = append(regularDrops, droppedIndex)
		}
	}

	// Remove replaced indexes from added list
	remainingAdded := []*ir.Index{}
	for _, addedIndex := range addedIndexes {
		if _, isReplacement := onlineReplacements[addedIndex.Name]; !isReplacement {
			remainingAdded = append(remainingAdded, addedIndex)
		}
	}

	// Drop indexes that are not being replaced
	for _, index := range regularDrops {
		sql := fmt.Sprintf("DROP INDEX IF EXISTS %s;", qualifyEntityName(index.Schema, index.Name, targetSchema))
		context := &diffContext{
			Type:                indexDiffType,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
			Source:              index,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}

	// Handle modified indexes
	for _, indexDiff := range modifiedIndexes {
		// Check if only comment changed
		structurallyEqual := indexesStructurallyEqual(indexDiff.Old, indexDiff.New)
		commentChanged := indexDiff.Old.Comment != indexDiff.New.Comment

		if structurallyEqual && commentChanged {
			// Only comment changed - generate COMMENT ON INDEX statement
			generateIndexComment(indexDiff.New, targetSchema, commentDiffType, DiffOperationAlter, collector)
		} else {
			// Structure changed - use online replacement approach
			dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s;", qualifyEntityName(indexDiff.Old.Schema, indexDiff.Old.Name, targetSchema))
			canonicalSQL := generateIndexSQL(indexDiff.New, targetSchema, false)

			statements := []SQLStatement{
				{
					SQL:                 dropSQL,
					CanRunInTransaction: true,
				},
				{
					SQL:                 canonicalSQL,
					CanRunInTransaction: true,
				},
			}

			alterContext := &diffContext{
				Type:                indexDiffType,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", indexDiff.New.Schema, indexDiff.New.Table, indexDiff.New.Name),
				Source:              indexDiff,
				CanRunInTransaction: true,
			}
			collector.collectStatements(alterContext, statements)
		}
	}

	// Process index replacements with online approach
	if len(onlineReplacements) > 0 {
		// Sort for deterministic order
		sortedOnlineIndexNames := make([]string, 0, len(onlineReplacements))
		for indexName := range onlineReplacements {
			sortedOnlineIndexNames = append(sortedOnlineIndexNames, indexName)
		}
		sort.Strings(sortedOnlineIndexNames)

		for _, indexName := range sortedOnlineIndexNames {
			newIndex := onlineReplacements[indexName]

			// Step 1: DROP old index, Step 2: CREATE new index
			dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s;", qualifyEntityName(newIndex.Schema, indexName, targetSchema))
			canonicalSQL := generateIndexSQL(newIndex, targetSchema, false)

			statements := []SQLStatement{
				{
					SQL:                 dropSQL,
					CanRunInTransaction: true,
				},
				{
					SQL:                 canonicalSQL,
					CanRunInTransaction: true,
				},
			}

			alterContext := &diffContext{
				Type:                indexDiffType,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", newIndex.Schema, newIndex.Table, indexName),
				Source:              newIndex,
				CanRunInTransaction: true,
			}
			collector.collectStatements(alterContext, statements)

			// Add index comment if present as a separate operation
			if newIndex.Comment != "" {
				generateIndexComment(newIndex, targetSchema, commentDiffType, DiffOperationCreate, collector)
			}
		}
	}

	// Create new indexes (not replacements)
	for _, index := range remainingAdded {
		canonicalSQL := generateIndexSQL(index, targetSchema, false)

		context := &diffContext{
			Type:                indexDiffType,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
			Source:              index,
			CanRunInTransaction: true,
		}

		collector.collect(context, canonicalSQL)

		// Add index comment if present
		if index.Comment != "" {
			generateIndexComment(index, targetSchema, commentDiffType, DiffOperationCreate, collector)
		}
	}
}

// generateIndexComment generates COMMENT ON INDEX statement
func generateIndexComment(
	index *ir.Index,
	targetSchema string,
	diffType DiffType,
	operation DiffOperation,
	collector *diffCollector,
) {
	indexName := qualifyEntityName(index.Schema, index.Name, targetSchema)
	var sql string
	if index.Comment == "" {
		sql = fmt.Sprintf("COMMENT ON INDEX %s IS NULL;", indexName)
	} else {
		sql = fmt.Sprintf("COMMENT ON INDEX %s IS %s;", indexName, quoteString(index.Comment))
	}

	context := &diffContext{
		Type:                diffType,
		Operation:           operation,
		Path:                fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
		Source:              index,
		CanRunInTransaction: true,
	}
	collector.collect(context, sql)
}
