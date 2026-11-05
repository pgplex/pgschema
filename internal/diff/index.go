package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pgplex/pgschema/ir"
)

// OnlineIndexTempNameSuffix marks the index built during an online rewrite.
const OnlineIndexTempNameSuffix = "_pgschema_new"

// maxIdentifierBytes is PostgreSQL's NAMEDATALEN-1: identifiers longer than this
// are truncated by the server.
const maxIdentifierBytes = 63

// OnlineIndexTempName returns the name of the index built during an online
// rewrite of the index called name.
//
// Appending the suffix to a long name can exceed PostgreSQL's 63-byte
// identifier limit. The server then truncates the candidate back to the
// canonical name, so CREATE INDEX CONCURRENTLY IF NOT EXISTS skips the build,
// the wait returns at once, and the final DROP/ALTER pair fails on a relation
// that is already gone. When the suffix does not fit, the canonical part is
// shortened and a digest of the full name takes the tail, so the candidate fits,
// differs from the canonical name, and stays distinct for names that share a
// long prefix.
func OnlineIndexTempName(name string) string {
	if len(name)+len(OnlineIndexTempNameSuffix) <= maxIdentifierBytes {
		return name + OnlineIndexTempNameSuffix
	}
	sum := sha256.Sum256([]byte(name))
	digest := hex.EncodeToString(sum[:4]) // 8 hex characters
	budget := maxIdentifierBytes - len(OnlineIndexTempNameSuffix) - len(digest) - 1
	stem := name
	if len(stem) > budget {
		stem = stem[:budget]
		// Do not cut a multi-byte character in half: the server counts bytes,
		// but a truncated rune would produce a name PostgreSQL rejects.
		for len(stem) > 0 && !utf8.ValidString(stem) {
			stem = stem[:len(stem)-1]
		}
	}
	return stem + "-" + digest + OnlineIndexTempNameSuffix
}

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

		generateCreateIndexSQL(index, targetSchema, collector, indexType, commentType, false)
	}
}

func generateCreateIndexSQL(index *ir.Index, targetSchema string, collector *diffCollector, indexType DiffType, commentType DiffType, only bool) {
	canonicalSQL := generateIndexSQLWithOptions(index, index.Name, targetSchema, false, collector.qualifySchema, only)

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
		indexName := qualifyEntityNameMode(index.Schema, index.Name, targetSchema, collector.qualifySchema)
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

// generateIndexSQL generates CREATE INDEX statement
func generateIndexSQL(index *ir.Index, targetSchema string, isConcurrent bool) string {
	return generateIndexSQLMode(index, targetSchema, isConcurrent, false)
}

// generateIndexSQLMode is like generateIndexSQL, but when qualifySchema is true the
// ON-table reference is always schema-qualified, even within the target schema.
func generateIndexSQLMode(index *ir.Index, targetSchema string, isConcurrent bool, qualifySchema bool) string {
	return generateIndexSQLWithNameMode(index, index.Name, targetSchema, isConcurrent, qualifySchema)
}

// generateIndexSQLWithName generates CREATE INDEX statement with custom name
func generateIndexSQLWithName(index *ir.Index, indexName string, targetSchema string, isConcurrent bool) string {
	return generateIndexSQLWithNameMode(index, indexName, targetSchema, isConcurrent, false)
}

// generateIndexSQLWithNameMode is like generateIndexSQLWithName, but when qualifySchema
// is true the ON-table reference is always schema-qualified, even within the target schema.
func generateIndexSQLWithNameMode(index *ir.Index, indexName string, targetSchema string, isConcurrent bool, qualifySchema bool) string {
	return generateIndexSQLWithOptions(index, indexName, targetSchema, isConcurrent, qualifySchema, false)
}

func generateIndexSQLWithOptions(index *ir.Index, indexName string, targetSchema string, isConcurrent bool, qualifySchema bool, only bool) string {
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

	if only {
		builder.WriteString("ONLY ")
	}

	// Table name with proper schema qualification
	tableName := getTableNameWithSchemaMode(index.Schema, index.Table, targetSchema, qualifySchema)
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

		// Add null ordering if non-default
		if col.NullOrder != "" {
			builder.WriteString(" ")
			builder.WriteString(col.NullOrder)
		}
	}
	builder.WriteString(")")

	// INCLUDE columns (non-key columns stored in the index)
	if len(index.IncludeColumns) > 0 {
		builder.WriteString(" INCLUDE (")
		for i, col := range index.IncludeColumns {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(col)
		}
		builder.WriteString(")")
	}

	// NULLS NOT DISTINCT for unique indexes (PostgreSQL 15+)
	if index.NullsNotDistinct && index.Type == ir.IndexTypeUnique {
		builder.WriteString(" NULLS NOT DISTINCT")
	}

	// Storage parameters
	if len(index.StorageParameters) > 0 {
		builder.WriteString(" WITH (")
		builder.WriteString(strings.Join(index.StorageParameters, ", "))
		builder.WriteString(")")
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
// Works for both table indexes and materialized view indexes.
// recreatedColumns names the columns applied as DROP COLUMN + ADD COLUMN; an
// index on one of them is gone by the time the index steps run, so it can
// never serve as a finished candidate.
func generateIndexModifications(
	droppedIndexes []*ir.Index,
	addedIndexes []*ir.Index,
	modifiedIndexes []*IndexDiff,
	recreatedColumns map[string]bool,
	targetSchema string,
	indexDiffType DiffType,
	commentDiffType DiffType,
	collector *diffCollector,
) {
	// Identify indexes that need online replacement (dropped and added with same name)
	onlineReplacements := make(map[string]*ir.Index)
	replacedIndexes := make(map[string]*ir.Index)
	regularDrops := []*ir.Index{}

	for _, droppedIndex := range droppedIndexes {
		foundReplacement := false
		for _, addedIndex := range addedIndexes {
			if droppedIndex.Name == addedIndex.Name {
				onlineReplacements[droppedIndex.Name] = addedIndex
				replacedIndexes[droppedIndex.Name] = droppedIndex
				foundReplacement = true
				break
			}
		}
		if !foundReplacement {
			regularDrops = append(regularDrops, droppedIndex)
		}
	}

	// A replacement whose candidate already exists, valid and matching the
	// desired definition, is resumed: the candidate is renamed into place
	// instead of being dropped and rebuilt. (#672)
	reusedCandidates := make(map[string]*ir.Index)
	candidateNames := make(map[string]string, len(onlineReplacements))
	for indexName := range onlineReplacements {
		candidateNames[OnlineIndexTempName(indexName)] = indexName
	}
	remainingDrops := []*ir.Index{}
	for _, droppedIndex := range regularDrops {
		indexName, isCandidate := candidateNames[droppedIndex.Name]
		if isCandidate && isReusableCandidate(droppedIndex, onlineReplacements[indexName], recreatedColumns) {
			reusedCandidates[indexName] = droppedIndex
			continue
		}
		remainingDrops = append(remainingDrops, droppedIndex)
	}
	regularDrops = remainingDrops

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

			if candidate := reusedCandidates[indexName]; candidate != nil {
				// The finished candidate takes the canonical name. Its comment
				// carries over with it, so only a differing comment is re-set.
				renameSQL := fmt.Sprintf("ALTER INDEX %s RENAME TO %s;",
					qualifyEntityName(candidate.Schema, candidate.Name, targetSchema), ir.QuoteIdentifier(indexName))
				statements := []SQLStatement{
					{SQL: dropSQL, CanRunInTransaction: true},
					{SQL: renameSQL, CanRunInTransaction: true},
				}
				alterContext := &diffContext{
					Type:      indexDiffType,
					Operation: DiffOperationAlter,
					Path:      fmt.Sprintf("%s.%s.%s", newIndex.Schema, newIndex.Table, indexName),
					Source: &IndexDiff{
						Old:             replacedIndexes[indexName],
						New:             newIndex,
						ReusedCandidate: candidate.Name,
					},
					CanRunInTransaction: true,
				}
				collector.collectStatements(alterContext, statements)
				if newIndex.Comment != candidate.Comment {
					generateIndexComment(newIndex, targetSchema, commentDiffType, DiffOperationAlter, collector)
				}
				continue
			}

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

// isReusableCandidate reports whether candidate, an index left behind by an
// earlier online rewrite of newIndex, can be renamed into place instead of
// being rebuilt. The name alone proves nothing: any index may occupy it. The
// candidate must sit on the same relation, be valid (an interrupted CREATE
// INDEX CONCURRENTLY leaves an invalid index of that name behind), and match
// the desired definition exactly. An index on a re-created column is dropped
// with the column before the index steps run and so cannot be reused.
func isReusableCandidate(candidate, newIndex *ir.Index, recreatedColumns map[string]bool) bool {
	if candidate.Schema != newIndex.Schema || candidate.Table != newIndex.Table {
		return false
	}
	if !candidate.IsValid {
		return false
	}
	if indexReferencesColumns(candidate, recreatedColumns) {
		return false
	}
	return indexesStructurallyEqual(candidate, newIndex)
}

// generateIndexComment generates COMMENT ON INDEX statement
func generateIndexComment(
	index *ir.Index,
	targetSchema string,
	diffType DiffType,
	operation DiffOperation,
	collector *diffCollector,
) {
	indexName := qualifyEntityNameMode(index.Schema, index.Name, targetSchema, collector.qualifySchema)
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
