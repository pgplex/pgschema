package ir

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/pgplex/pgschema/ir/queries"
	"golang.org/x/sync/errgroup"
)

// PostgreSQL trigger type bitmask constants from pg_trigger.tgtype
// Reference: https://github.com/postgres/postgres/blob/master/src/include/catalog/pg_trigger.h
const (
	triggerTypeRow      = 1 << 0 // TRIGGER_TYPE_ROW - row-level trigger
	triggerTypeBefore   = 1 << 1 // TRIGGER_TYPE_BEFORE - BEFORE timing
	triggerTypeInsert   = 1 << 2 // TRIGGER_TYPE_INSERT - INSERT event
	triggerTypeDelete   = 1 << 3 // TRIGGER_TYPE_DELETE - DELETE event
	triggerTypeUpdate   = 1 << 4 // TRIGGER_TYPE_UPDATE - UPDATE event
	triggerTypeTruncate = 1 << 5 // TRIGGER_TYPE_TRUNCATE - TRUNCATE event
	triggerTypeInstead  = 1 << 6 // TRIGGER_TYPE_INSTEAD - INSTEAD OF timing
)

// PostgreSQL sequence type maximum value constants
const (
	smallintMaxValue = 32767               // Maximum value for smallint sequences
	integerMaxValue  = 2147483647          // Maximum value for integer sequences
	bigintMaxValue   = 9223372036854775807 // Maximum value for bigint sequences (math.MaxInt64)
)

// Inspector builds IR from database queries
type Inspector struct {
	db           *sql.DB
	queries      *queries.Queries
	ignoreConfig *IgnoreConfig
}

// NewInspector creates a new schema inspector with optional ignore configuration
func NewInspector(db *sql.DB, ignoreConfig *IgnoreConfig) *Inspector {
	return &Inspector{
		db:           db,
		queries:      queries.New(db),
		ignoreConfig: ignoreConfig,
	}
}

// BuildIR builds the schema IR from the database for a specific schema
func (i *Inspector) BuildIR(ctx context.Context, targetSchema string) (*IR, error) {
	schema := NewIR()

	// Sequential prerequisites
	if err := i.buildMetadata(ctx, schema); err != nil {
		return nil, fmt.Errorf("failed to build metadata: %w", err)
	}

	if err := i.validateSchemaExists(ctx, targetSchema); err != nil {
		return nil, err
	}

	if err := i.buildSchemas(ctx, schema, targetSchema); err != nil {
		return nil, fmt.Errorf("failed to build schemas: %w", err)
	}

	if err := i.buildTables(ctx, schema, targetSchema); err != nil {
		return nil, fmt.Errorf("failed to build tables: %w", err)
	}

	// Concurrent Group 1: Table Details
	group1 := queryGroup{
		name: "table details",
		funcs: []func(context.Context, *IR, string) error{
			i.buildColumns,
			i.buildConstraints,
			i.buildPartitions,
		},
	}

	// Concurrent Group 2: Independent Objects
	group2 := queryGroup{
		name: "independent objects",
		funcs: []func(context.Context, *IR, string) error{
			i.buildSequences,
			i.buildFunctions,
			i.buildProcedures,
			i.buildAggregates,
			i.buildTypes,
			i.buildDefaultPrivileges,
			i.buildPrivileges,
			i.buildColumnPrivileges,
		},
	}

	// Concurrent Group 3: View and table-dependent objects (views must load first)
	group3 := queryGroup{
		name: "views",
		funcs: []func(context.Context, *IR, string) error{
			i.buildViews,
		},
	}

	// Group 4: Objects that depend on both tables AND views (must run after views are loaded)
	group4 := queryGroup{
		name: "table-and-view-dependent objects",
		funcs: []func(context.Context, *IR, string) error{
			i.buildTriggers,
			i.buildRLSPolicies,
		},
	}

	// Execute groups concurrently where possible
	var eg errgroup.Group

	// Group 1 & 2 can run in parallel
	eg.Go(func() error {
		return i.executeConcurrentGroup(ctx, schema, targetSchema, group1)
	})

	eg.Go(func() error {
		return i.executeConcurrentGroup(ctx, schema, targetSchema, group2)
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Build function dependencies after functions are loaded
	if err := i.buildFunctionDependencies(ctx, schema, targetSchema); err != nil {
		return nil, err
	}

	// Group 3 runs after table details are loaded (views must be loaded before triggers)
	if err := i.executeConcurrentGroup(ctx, schema, targetSchema, group3); err != nil {
		return nil, err
	}

	// Group 4 runs after views are loaded (triggers can reference views for INSTEAD OF triggers)
	if err := i.executeConcurrentGroup(ctx, schema, targetSchema, group4); err != nil {
		return nil, err
	}

	// Build indexes after views are loaded (indexes can reference materialized views)
	if err := i.buildIndexes(ctx, schema, targetSchema); err != nil {
		return nil, fmt.Errorf("failed to build indexes: %w", err)
	}

	// Normalize the IR
	normalizeIR(schema)

	return schema, nil
}

// queryGroup represents a group of queries that can be executed concurrently
type queryGroup struct {
	name  string
	funcs []func(context.Context, *IR, string) error
}

// executeConcurrentGroup executes a group of functions concurrently
func (i *Inspector) executeConcurrentGroup(ctx context.Context, schema *IR, targetSchema string, group queryGroup) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(group.funcs))

	for _, fn := range group.funcs {
		wg.Add(1)
		go func(f func(context.Context, *IR, string) error) {
			defer wg.Done()
			if err := f(ctx, schema, targetSchema); err != nil {
				errChan <- err
			}
		}(fn)
	}

	wg.Wait()
	close(errChan)

	// Collect errors
	for err := range errChan {
		if err != nil {
			return fmt.Errorf("%s: %w", group.name, err)
		}
	}
	return nil
}

func (i *Inspector) buildMetadata(ctx context.Context, schema *IR) error {
	var dbVersion string
	if err := i.db.QueryRowContext(ctx, "SELECT version()").Scan(&dbVersion); err != nil {
		return err
	}

	// Extract version number from the version string
	if strings.Contains(dbVersion, "PostgreSQL") {
		parts := strings.Fields(dbVersion)
		if len(parts) >= 2 {
			dbVersion = "PostgreSQL " + parts[1]
		}
	}

	schema.Metadata = Metadata{
		DatabaseVersion: dbVersion,
	}

	return nil
}

func (i *Inspector) buildSchemas(ctx context.Context, schema *IR, targetSchema string) error {
	// Use the schema-specific query to prefilter at the database level
	schemaName, err := i.queries.GetSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	schema.getOrCreateSchema(i.safeInterfaceToString(schemaName))

	return nil
}

func (i *Inspector) buildTables(ctx context.Context, schema *IR, targetSchema string) error {
	tables, err := i.queries.GetTablesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, table := range tables {
		schemaName := i.safeInterfaceToString(table.TableSchema)
		tableName := i.safeInterfaceToString(table.TableName)
		tableType := i.safeInterfaceToString(table.TableType)
		comment := ""
		if table.TableComment.Valid {
			comment = table.TableComment.String
		}

		// No need to filter by schema since query is already schema-specific

		// Check if table should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreTable(tableName) {
			continue
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		// Skip views as they are handled by buildViews function
		if tableType == "VIEW" {
			continue
		}

		var tType TableType
		switch tableType {
		case "BASE TABLE":
			tType = TableTypeBase
		default:
			tType = TableTypeBase
		}

		t := &Table{
			Schema:      schemaName,
			Name:        tableName,
			Type:        tType,
			Comment:     comment,
			Columns:     []*Column{},
			Constraints: make(map[string]*Constraint),
			Indexes:     make(map[string]*Index),
			Triggers:    make(map[string]*Trigger),
			Policies:    make(map[string]*RLSPolicy),
		}

		dbSchema.SetTable(tableName, t)
	}

	return nil
}

func (i *Inspector) buildColumns(ctx context.Context, schema *IR, targetSchema string) error {
	columns, err := i.queries.GetColumnsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, col := range columns {
		schemaName := i.safeInterfaceToString(col.TableSchema)
		tableName := i.safeInterfaceToString(col.TableName)
		columnName := i.safeInterfaceToString(col.ColumnName)
		comment := ""
		if col.ColumnComment.Valid {
			comment = col.ColumnComment.String
		}

		// No need to filter by schema since query is already schema-specific

		dbSchema := schema.getOrCreateSchema(schemaName)
		table, exists := dbSchema.Tables[tableName]
		if !exists {
			continue // Skip columns for non-existent tables
		}

		// Get the resolved type - schema prefix and type normalization is now handled during read time
		resolvedType := i.safeInterfaceToString(col.ResolvedType)
		// Map internal PostgreSQL types to standard SQL types
		dataType := normalizePostgreSQLType(resolvedType)

		column := &Column{
			Name:       columnName,
			Position:   i.safeInterfaceToInt(col.OrdinalPosition, 0),
			DataType:   dataType,
			IsNullable: i.safeInterfaceToString(col.IsNullable) == "YES",
			Comment:    comment,
		}

		// Handle generated columns first
		isGeneratedColumn := i.safeInterfaceToString(col.Attgenerated) == "s"
		if isGeneratedColumn {
			column.IsGenerated = true
			if generatedExpr := i.safeInterfaceToString(col.GeneratedExpr); generatedExpr != "" {
				column.GeneratedExpr = &generatedExpr
			}
		}

		// Handle default value - keep original value as stored in database
		// Don't set default values for generated columns
		if !isGeneratedColumn {
			if defaultVal := i.safeInterfaceToString(col.ColumnDefault); defaultVal != "" && defaultVal != "<nil>" {
				column.DefaultValue = &defaultVal
			}
		}

		// Handle max length
		if maxLen := i.safeInterfaceToInt64(col.CharacterMaximumLength, -1); maxLen > 0 {
			maxLenInt := int(maxLen)
			column.MaxLength = &maxLenInt
		}

		// Handle numeric precision and scale
		if precision := i.safeInterfaceToInt64(col.NumericPrecision, -1); precision > 0 {
			precisionInt := int(precision)
			column.Precision = &precisionInt
		}

		if scale := i.safeInterfaceToInt64(col.NumericScale, -1); scale >= 0 {
			scaleInt := int(scale)
			column.Scale = &scaleInt
		}

		// Handle identity columns
		if i.safeInterfaceToString(col.IsIdentity) == "YES" {
			identity := &Identity{
				Generation: i.safeInterfaceToString(col.IdentityGeneration),
				Cycle:      i.safeInterfaceToString(col.IdentityCycle) == "YES",
			}

			if start := i.safeInterfaceToInt64(col.IdentityStart, -1); start >= 0 {
				identity.Start = &start
			}

			if increment := i.safeInterfaceToInt64(col.IdentityIncrement, -1); increment >= 0 {
				identity.Increment = &increment
			}

			if maximum := i.safeInterfaceToInt64(col.IdentityMaximum, -1); maximum >= 0 {
				identity.Maximum = &maximum
			}

			if minimum := i.safeInterfaceToInt64(col.IdentityMinimum, -1); minimum >= 0 {
				identity.Minimum = &minimum
			}

			column.Identity = identity
		}

		// Check if column already exists to avoid duplicates
		columnExists := false
		for _, existingCol := range table.Columns {
			if existingCol.Name == columnName {
				columnExists = true
				break
			}
		}

		// Only add column if it doesn't already exist
		if !columnExists {
			table.Columns = append(table.Columns, column)
		}
	}

	return nil
}

func (i *Inspector) buildPartitions(ctx context.Context, schema *IR, targetSchema string) error {
	// Use the schema-specific query to prefilter at the database level
	partitions, err := i.queries.GetPartitionedTablesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, partition := range partitions {
		schemaName := partition.TableSchema
		tableName := partition.TableName
		partitionStrategy := ""
		if partition.PartitionStrategy.Valid {
			partitionStrategy = partition.PartitionStrategy.String
		}
		partitionKey := ""
		if partition.PartitionKey.Valid {
			partitionKey = partition.PartitionKey.String
		}

		// No need to filter by schema since query is already schema-specific

		dbSchema := schema.getOrCreateSchema(schemaName)
		table, exists := dbSchema.Tables[tableName]
		if !exists {
			continue // Skip partitions for non-existent tables
		}

		table.IsPartitioned = true
		table.PartitionStrategy = partitionStrategy
		table.PartitionKey = partitionKey
	}

	return nil
}

func (i *Inspector) buildConstraints(ctx context.Context, schema *IR, targetSchema string) error {
	constraints, err := i.queries.GetConstraintsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Group constraints by key to handle composite constraints
	type constraintKey struct {
		schema string
		table  string
		name   string
	}
	constraintGroups := make(map[constraintKey]*Constraint)

	for _, constraint := range constraints {
		schemaName := constraint.TableSchema
		tableName := constraint.TableName
		constraintName := constraint.ConstraintName
		constraintType := ""
		if constraint.ConstraintType.Valid {
			constraintType = constraint.ConstraintType.String
		}

		// Extract column name from sql.NullString
		columnName := ""
		if constraint.ColumnName.Valid {
			columnName = constraint.ColumnName.String
		}

		if columnName == "" || columnName == "<nil>" {
			// CHECK and EXCLUDE constraints may have no columns (e.g., CHECK (FALSE))
			// Only skip non-CHECK/EXCLUDE constraints without columns
			if constraintType != "CHECK" && constraintType != "EXCLUDE" {
				continue
			}
		}

		key := constraintKey{
			schema: schemaName,
			table:  tableName,
			name:   constraintName,
		}

		// Get or create constraint
		c, exists := constraintGroups[key]
		if !exists {
			var cType ConstraintType
			switch constraintType {
			case "PRIMARY KEY":
				cType = ConstraintTypePrimaryKey
			case "UNIQUE":
				cType = ConstraintTypeUnique
			case "FOREIGN KEY":
				cType = ConstraintTypeForeignKey
			case "CHECK":
				cType = ConstraintTypeCheck
			case "EXCLUDE":
				cType = ConstraintTypeExclusion
			default:
				continue // Skip unknown constraint types
			}

			c = &Constraint{
				Schema:     schemaName,
				Table:      tableName,
				Name:       constraintName,
				Type:       cType,
				Columns:    []*ConstraintColumn{},
				NoInherit:  constraint.NoInherit,
				IsTemporal: constraint.IsPeriod, // PG18 temporal constraint (WITHOUT OVERLAPS / PERIOD)
			}

			// Handle foreign key references
			if cType == ConstraintTypeForeignKey {
				if refSchema := i.safeInterfaceToString(constraint.ForeignTableSchema); refSchema != "" && refSchema != "<nil>" {
					c.ReferencedSchema = refSchema
				}
				if refTable := i.safeInterfaceToString(constraint.ForeignTableName); refTable != "" && refTable != "<nil>" {
					c.ReferencedTable = refTable
				}
				if deleteRule := i.safeInterfaceToString(constraint.DeleteRule); deleteRule != "" && deleteRule != "<nil>" {
					c.DeleteRule = deleteRule
				}
				if updateRule := i.safeInterfaceToString(constraint.UpdateRule); updateRule != "" && updateRule != "<nil>" {
					c.UpdateRule = updateRule
				}
				// Handle deferrable attributes for foreign key constraints
				c.Deferrable = constraint.Deferrable
				c.InitiallyDeferred = constraint.InitiallyDeferred
			}

			// Handle check constraints
			if cType == ConstraintTypeCheck {
				if checkClause := i.safeInterfaceToString(constraint.CheckClause); checkClause != "" && checkClause != "<nil>" {
					// Skip system-generated NOT NULL constraints as they're redundant with column definitions
					if strings.Contains(checkClause, "IS NOT NULL") {
						continue
					}

					// Use CheckClause as-is from PostgreSQL's pg_get_constraintdef(c.oid, true)
					// For NOT VALID constraints, PostgreSQL includes " NOT VALID" suffix in the output
					// We keep it as-is since we always output CHECK constraints as named table-level constraints
					c.CheckClause = checkClause
				}
			}

			// Handle exclusion constraints
			if cType == ConstraintTypeExclusion {
				if exclDef := i.safeInterfaceToString(constraint.ExclusionDefinition); exclDef != "" && exclDef != "<nil>" {
					c.ExclusionDefinition = exclDef
				}
			}

			// Set validation state from database
			c.IsValid = constraint.IsValid

			constraintGroups[key] = c
		}

		// Add column to constraint (skip for column-less constraints like CHECK (FALSE))
		if columnName != "" && columnName != "<nil>" {
			// Get column position in constraint
			position := i.getConstraintColumnPosition(ctx, schemaName, constraintName, columnName)

			// Check if column already exists in constraint to avoid duplicates
			columnExists := false
			for _, existingCol := range c.Columns {
				if existingCol.Name == columnName {
					columnExists = true
					break
				}
			}

			// Add column to constraint only if it doesn't exist
			if !columnExists {
				constraintCol := &ConstraintColumn{
					Name:     columnName,
					Position: position,
				}
				c.Columns = append(c.Columns, constraintCol)
			}

			// Handle foreign key referenced columns
			if c.Type == ConstraintTypeForeignKey {
				if refColumnName := i.safeInterfaceToString(constraint.ForeignColumnName); refColumnName != "" && refColumnName != "<nil>" {
					// Check if referenced column already exists to avoid duplicates
					refColumnExists := false
					for _, existingRefCol := range c.ReferencedColumns {
						if existingRefCol.Name == refColumnName {
							refColumnExists = true
							break
						}
					}

					// Add referenced column only if it doesn't exist
					if !refColumnExists {
						// Use the local column's constraint position for the referenced column.
						// The local and referenced columns are paired together in the FK definition,
						// so they must have the same position to maintain correct ordering.
						// Note: ForeignOrdinalPosition from the query is fa.attnum (column position
						// in the foreign table's definition), which is wrong - we need the constraint
						// array position, which is the same as the local column's position.
						refConstraintCol := &ConstraintColumn{
							Name:     refColumnName,
							Position: position, // Use local column's constraint position
						}
						c.ReferencedColumns = append(c.ReferencedColumns, refConstraintCol)
					}
				}
			}
		}
	}

	// Build a mapping of partition tables to their parent's partition keys
	partitionMapping := i.buildPartitionMapping(ctx, schema, targetSchema)

	// Add constraints to tables
	for key, constraint := range constraintGroups {
		dbSchema := schema.getOrCreateSchema(key.schema)
		table, exists := dbSchema.Tables[key.table]
		if exists {
			// Sort constraint columns by their position to preserve original order from database
			// This ensures constraints maintain the correct column order as defined in PostgreSQL
			if requiresPositionSorting(constraint.Type) {
				sort.Slice(constraint.Columns, func(i, j int) bool {
					return constraint.Columns[i].Position < constraint.Columns[j].Position
				})

				// Also sort referenced columns for foreign keys
				if constraint.Type == ConstraintTypeForeignKey && len(constraint.ReferencedColumns) > 0 {
					sort.Slice(constraint.ReferencedColumns, func(i, j int) bool {
						return constraint.ReferencedColumns[i].Position < constraint.ReferencedColumns[j].Position
					})
				}
			}

			table.Constraints[key.name] = constraint

			// For partitioned tables, ensure primary key columns are ordered with partition key first
			// This special handling overrides the position-based sorting for partitioned tables
			if constraint.Type == ConstraintTypePrimaryKey && table.IsPartitioned && table.PartitionKey != "" {
				i.sortPrimaryKeyColumnsForPartitionedTable(constraint, table.PartitionKey)
			}

			// For partition tables (children of partitioned tables), use parent's partition key
			if constraint.Type == ConstraintTypePrimaryKey && !table.IsPartitioned {
				if parentPartitionKey, isPartitionTable := partitionMapping[key.table]; isPartitionTable {
					i.sortPrimaryKeyColumnsForPartitionedTable(constraint, parentPartitionKey)
				}
			}
		}
	}

	return nil
}

// requiresPositionSorting returns true if the constraint type requires columns to be sorted by position
func requiresPositionSorting(constraintType ConstraintType) bool {
	switch constraintType {
	case ConstraintTypeUnique, ConstraintTypePrimaryKey, ConstraintTypeForeignKey:
		return true
	default:
		return false
	}
}

// buildPartitionMapping builds a mapping from partition table names to their parent's partition keys
func (i *Inspector) buildPartitionMapping(ctx context.Context, schema *IR, targetSchema string) map[string]string {
	partitionMapping := make(map[string]string)

	// Get partition children information
	partitionChildren, err := i.queries.GetPartitionChildren(ctx)
	if err != nil {
		// If we can't get partition info, return empty mapping
		return partitionMapping
	}

	for _, child := range partitionChildren {
		// Only process children in the target schema
		if child.ChildSchema != targetSchema {
			continue
		}

		childTable := child.ChildTable
		parentTable := child.ParentTable

		// Find the parent table's partition key
		dbSchema := schema.getOrCreateSchema(targetSchema)
		if parentTableInfo, exists := dbSchema.Tables[parentTable]; exists && parentTableInfo.IsPartitioned {
			partitionMapping[childTable] = parentTableInfo.PartitionKey
		}
	}

	return partitionMapping
}

// sortPrimaryKeyColumnsForPartitionedTable sorts primary key constraint columns
// to ensure partition key columns come first
func (i *Inspector) sortPrimaryKeyColumnsForPartitionedTable(constraint *Constraint, partitionKey string) {
	if constraint.Type != ConstraintTypePrimaryKey || len(constraint.Columns) <= 1 {
		return
	}

	// Parse partition key to handle multi-column partitions
	partitionColumns := make(map[string]bool)
	for _, col := range strings.Split(partitionKey, ",") {
		partitionColumns[strings.TrimSpace(col)] = true
	}

	// Separate partition columns from non-partition columns
	var partitionCols []*ConstraintColumn
	var nonPartitionCols []*ConstraintColumn

	for _, col := range constraint.Columns {
		if partitionColumns[col.Name] {
			partitionCols = append(partitionCols, col)
		} else {
			nonPartitionCols = append(nonPartitionCols, col)
		}
	}

	// Sort partition columns by their position to maintain consistent ordering
	sort.Slice(partitionCols, func(i, j int) bool {
		return partitionCols[i].Position < partitionCols[j].Position
	})

	// Sort non-partition columns by their position
	sort.Slice(nonPartitionCols, func(i, j int) bool {
		return nonPartitionCols[i].Position < nonPartitionCols[j].Position
	})

	// Rebuild the columns list with partition columns first
	constraint.Columns = append(partitionCols, nonPartitionCols...)
}

func (i *Inspector) buildIndexes(ctx context.Context, schema *IR, targetSchema string) error {
	// Get indexes for the target schema
	indexes, err := i.queries.GetIndexesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, indexRow := range indexes {
		schemaName := indexRow.Schemaname
		tableName := indexRow.Tablename
		indexName := indexRow.Indexname

		dbSchema := schema.getOrCreateSchema(schemaName)

		// Extract values with null safety
		isUnique := indexRow.IsUnique
		isPrimary := indexRow.IsPrimary
		isPartial := indexRow.IsPartial.Valid && indexRow.IsPartial.Bool
		hasExpressions := indexRow.HasExpressions.Valid && indexRow.HasExpressions.Bool
		method := indexRow.Method

		// Determine index type based on properties
		indexType := IndexTypeRegular
		if isPrimary {
			indexType = IndexTypePrimary
		} else if isUnique {
			indexType = IndexTypeUnique
		}

		// Extract comment
		comment := ""
		if indexRow.IndexComment.Valid {
			comment = indexRow.IndexComment.String
		}

		index := &Index{
			Schema:       schemaName,
			Table:        tableName,
			Name:         indexName,
			Type:         indexType,
			Method:       method,
			IsPartial:    isPartial,
			IsExpression: hasExpressions,
			Where:        "",
			Comment:      comment,
			Columns:      []*IndexColumn{},
		}

		// Check for NULLS NOT DISTINCT (PostgreSQL 15+)
		if indexRow.Indexdef.Valid && strings.Contains(indexRow.Indexdef.String, "NULLS NOT DISTINCT") {
			index.NullsNotDistinct = true
		}

		// Set WHERE clause for partial indexes
		if isPartial && indexRow.PartialPredicate.Valid {
			// Use the predicate as-is from pg_get_expr, which already has proper formatting
			index.Where = indexRow.PartialPredicate.String
		}

		// Extract columns directly from query results
		// The query uses pg_get_indexdef(indexrelid, column_position, true) for each column
		// and extracts ASC/DESC from the indoption array
		// and operator class names from pg_index.indclass joined with pg_opclass
		for idx := 0; idx < len(indexRow.ColumnDefinitions); idx++ {
			columnName := indexRow.ColumnDefinitions[idx]
			direction := "ASC" // Default
			if idx < len(indexRow.ColumnDirections) {
				direction = indexRow.ColumnDirections[idx]
			}

			// Get operator class from the ColumnOpclasses array
			operatorClass := ""
			if idx < len(indexRow.ColumnOpclasses) {
				operatorClass = indexRow.ColumnOpclasses[idx]
			}

			indexColumn := &IndexColumn{
				Name:      columnName,
				Position:  idx + 1,
				Direction: direction,
				Operator:  operatorClass,
			}

			index.Columns = append(index.Columns, indexColumn)
		}

		// Add index to table or materialized view
		if table, exists := dbSchema.Tables[tableName]; exists {
			table.Indexes[indexName] = index
		} else if view, exists := dbSchema.Views[tableName]; exists && view.Materialized {
			// Initialize Indexes map if nil
			if view.Indexes == nil {
				view.Indexes = make(map[string]*Index)
			}
			view.Indexes[indexName] = index
		}
	}

	return nil
}

func (i *Inspector) buildSequences(ctx context.Context, schema *IR, targetSchema string) error {
	sequences, err := i.queries.GetSequencesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, seq := range sequences {
		schemaName := seq.SequenceSchema.String
		sequenceName := seq.SequenceName.String

		// Check if sequence should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreSequence(sequenceName) {
			continue
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		// Set empty DataType for sequences that use PostgreSQL's implicit bigint default
		dataType := i.safeInterfaceToString(seq.DataType)
		if dataType == "bigint" {
			// Check if this is a default bigint by looking at min/max values
			// Default bigint sequences have min_value=1 and max_value=bigintMaxValue
			if seq.MinimumValue.Valid && seq.MinimumValue.Int64 == 1 &&
				seq.MaximumValue.Valid && seq.MaximumValue.Int64 == bigintMaxValue {
				dataType = "" // This means it was not explicitly specified
			}
		}

		sequence := &Sequence{
			Schema:      schemaName,
			Name:        sequenceName,
			DataType:    dataType,
			StartValue:  seq.StartValue.Int64,
			Increment:   seq.Increment.Int64,
			CycleOption: seq.CycleOption.Bool,
		}

		// Set default values if not valid
		if !seq.StartValue.Valid {
			sequence.StartValue = 1
		}
		if !seq.Increment.Valid {
			sequence.Increment = 1
		}

		// Only set MinValue/MaxValue if they differ from the data type defaults
		if seq.MinimumValue.Valid {
			minVal := seq.MinimumValue.Int64
			// Only set if not the default (1) for this data type
			if minVal != 1 {
				sequence.MinValue = &minVal
			}
		}

		if seq.MaximumValue.Valid {
			maxVal := seq.MaximumValue.Int64
			defaultMax := getSequenceMaxValueForType(dataType)
			// Only set if not the default for this data type
			if maxVal != defaultMax {
				sequence.MaxValue = &maxVal
			}
		}

		// Set cache value if it's different from default (1)
		if seq.CacheSize.Valid && seq.CacheSize.Int64 != 1 {
			cacheVal := seq.CacheSize.Int64
			sequence.Cache = &cacheVal
		}

		// Set ownership information
		if seq.OwnedByTable.Valid {
			sequence.OwnedByTable = seq.OwnedByTable.String
		}
		if seq.OwnedByColumn.Valid {
			sequence.OwnedByColumn = seq.OwnedByColumn.String
		}

		// Skip sequences that are owned by identity columns
		// Identity sequences should be managed through the identity column, not as separate sequences
		if sequence.OwnedByTable != "" && sequence.OwnedByColumn != "" {
			// Check if the owning column is an identity column
			if i.isIdentityColumn(ctx, seq.SequenceSchema.String, sequence.OwnedByTable, sequence.OwnedByColumn) {
				// Skip this sequence - it's managed by the identity column
				continue
			}
		}

		dbSchema.SetSequence(sequenceName, sequence)
	}

	return nil
}

// isIdentityColumn checks if a column is an identity column
func (i *Inspector) isIdentityColumn(ctx context.Context, schemaName, tableName, columnName string) bool {
	query := `
		SELECT is_identity 
		FROM information_schema.columns 
		WHERE table_schema = $1 
		  AND table_name = $2 
		  AND column_name = $3`

	var isIdentity string
	err := i.db.QueryRowContext(ctx, query, schemaName, tableName, columnName).Scan(&isIdentity)
	if err != nil {
		return false
	}

	return isIdentity == "YES"
}

func (i *Inspector) buildFunctions(ctx context.Context, schema *IR, targetSchema string) error {
	functions, err := i.queries.GetFunctionsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, fn := range functions {
		schemaName := i.safeInterfaceToString(fn.RoutineSchema)
		functionName := i.safeInterfaceToString(fn.RoutineName)
		comment := ""
		if fn.FunctionComment.Valid {
			comment = fn.FunctionComment.String
		}
		signature := i.safeInterfaceToString(fn.FunctionSignature)

		// Check if function should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreFunction(functionName) {
			continue
		}

		// Get function definition from pg_get_functiondef
		definition := i.safeInterfaceToString(fn.RoutineDefinition)

		dbSchema := schema.getOrCreateSchema(schemaName)

		// Handle volatility
		volatility := i.safeInterfaceToString(fn.Volatility)

		// Handle strictness
		isStrict := fn.IsStrict

		// Handle security definer
		isSecurityDefiner := fn.IsSecurityDefiner

		// Handle leakproof
		isLeakproof := fn.IsLeakproof

		// Handle parallel mode
		parallelMode := ""
		proparallel := i.safeInterfaceToString(fn.ParallelMode)
		switch proparallel {
		case "s":
			parallelMode = "SAFE"
		case "r":
			parallelMode = "RESTRICTED"
		case "u":
			parallelMode = "UNSAFE"
		default:
			parallelMode = "UNSAFE" // Defensive default
		}

		// Parse parameters from the complete signature provided by pg_get_function_arguments()
		// This signature includes all parameter information including modes, names, types, and defaults
		parameters := i.parseParametersFromSignature(signature, schemaName)

		// Handle search_path
		searchPath := ""
		if fn.SearchPath.Valid {
			searchPath = fn.SearchPath.String
		}

		function := &Function{
			Schema:            schemaName,
			Name:              functionName,
			Definition:        definition,
			ReturnType:        i.safeInterfaceToString(fn.DataType),
			Language:          i.safeInterfaceToString(fn.ExternalLanguage),
			Comment:           comment,
			Parameters:        parameters,
			Volatility:        volatility,
			IsStrict:          isStrict,
			IsSecurityDefiner: isSecurityDefiner,
			IsLeakproof:       isLeakproof,
			Parallel:          parallelMode,
			SearchPath:        searchPath,
		}

		// Use name(arguments) as key to support function overloading
		// This allows multiple functions with the same name but different signatures
		functionKey := functionName + "(" + function.GetArguments() + ")"
		dbSchema.SetFunction(functionKey, function)
	}

	return nil
}

func (i *Inspector) buildFunctionDependencies(ctx context.Context, schema *IR, targetSchema string) error {
	deps, err := i.queries.GetFunctionDependencies(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	dbSchema := schema.Schemas[targetSchema]
	if dbSchema == nil {
		return nil
	}

	// Build a map of dependencies by dependent function key
	depMap := make(map[string][]string)
	for _, dep := range deps {
		dependentArgs := ""
		if dep.DependentArgs.Valid {
			dependentArgs = dep.DependentArgs.String
		}
		dependentKey := dep.DependentName + "(" + dependentArgs + ")"

		referencedArgs := ""
		if dep.ReferencedArgs.Valid {
			referencedArgs = dep.ReferencedArgs.String
		}

		// Store as schema.name(args) for cross-schema support
		referencedKey := dep.ReferencedSchema + "." + dep.ReferencedName + "(" + referencedArgs + ")"
		depMap[dependentKey] = append(depMap[dependentKey], referencedKey)
	}

	// Assign dependencies to functions
	for funcKey, fn := range dbSchema.Functions {
		if deps, ok := depMap[funcKey]; ok {
			fn.Dependencies = deps
		}
	}

	return nil
}

// splitParameterString splits a parameter string by commas, but respects quotes,
// parentheses, and brackets. This handles complex defaults like '{1,2,3}' or '{"key": "value"}'
func splitParameterString(signature string) []string {
	var params []string
	var current strings.Builder
	depth := 0       // Track nesting depth of (), [], {}
	inQuote := false // Track if we're inside a string literal

	i := 0
	for i < len(signature) {
		ch := rune(signature[i])

		switch ch {
		case '\'':
			// Toggle quote state, but handle escaped quotes
			if !inQuote {
				inQuote = true
				current.WriteRune(ch)
				i++
			} else {
				// Check if this is an escaped quote (two single quotes)
				if i+1 < len(signature) && signature[i+1] == '\'' {
					current.WriteRune(ch)
					current.WriteRune('\'')
					i += 2 // Skip both quotes
				} else {
					inQuote = false
					current.WriteRune(ch)
					i++
				}
			}
		case '(', '[', '{':
			if !inQuote {
				depth++
			}
			current.WriteRune(ch)
			i++
		case ')', ']', '}':
			if !inQuote {
				depth--
			}
			current.WriteRune(ch)
			i++
		case ',':
			if !inQuote && depth == 0 {
				// This comma is a parameter separator
				params = append(params, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				// This comma is inside quotes or nested structure
				current.WriteRune(ch)
			}
			i++
		default:
			current.WriteRune(ch)
			i++
		}
	}

	// Add the last parameter
	if current.Len() > 0 {
		params = append(params, strings.TrimSpace(current.String()))
	}

	return params
}

// parseParametersFromSignature parses function signature string into Parameter structs
// Example signature: "order_id integer, discount_percent numeric DEFAULT 0"
// Or with modes: "IN order_id integer, OUT result integer"
func (i *Inspector) parseParametersFromSignature(signature string, routineSchema string) []*Parameter {
	if signature == "" {
		return nil
	}

	var parameters []*Parameter
	position := 1

	// Split by comma to get individual parameters (smart split that respects quotes/brackets)
	paramStrings := splitParameterString(signature)
	for _, paramStr := range paramStrings {
		paramStr = strings.TrimSpace(paramStr)
		if paramStr == "" {
			continue
		}

		param := &Parameter{
			Mode:     "IN", // Default mode
			Position: position,
		}

		// Look for DEFAULT clause first
		defaultIdx := strings.Index(strings.ToUpper(paramStr), " DEFAULT ")
		if defaultIdx != -1 {
			// Extract default value
			defaultValue := strings.TrimSpace(paramStr[defaultIdx+9:]) // " DEFAULT " is 9 chars
			param.DefaultValue = &defaultValue
			paramStr = strings.TrimSpace(paramStr[:defaultIdx])
		}

		// Split into parts and check for mode prefix
		parts := strings.Fields(paramStr)
		if len(parts) == 0 {
			continue
		}

		// Check if first part is a mode keyword (IN, OUT, INOUT, VARIADIC, TABLE)
		firstPart := strings.ToUpper(parts[0])
		startIdx := 0
		if firstPart == "IN" || firstPart == "OUT" || firstPart == "INOUT" || firstPart == "VARIADIC" || firstPart == "TABLE" {
			param.Mode = firstPart
			startIdx = 1
		}

		// Parse name and type from remaining parts
		remainingParts := parts[startIdx:]
		if len(remainingParts) >= 2 {
			param.Name = remainingParts[0]
			param.DataType = strings.Join(remainingParts[1:], " ")
		} else if len(remainingParts) == 1 {
			// Only type, no name
			param.DataType = remainingParts[0]
		}

		// Normalize type by stripping schema prefix if it matches the routine's schema
		// This ensures consistent comparison between database and source SQL representations
		param.DataType = i.stripSameSchemaPrefix(param.DataType, routineSchema)

		parameters = append(parameters, param)
		position++
	}

	return parameters
}

// stripSameSchemaPrefix removes schema qualification from a type name if the schema
// matches the routine's schema. This normalizes PostgreSQL's behavior of returning
// schema-qualified type names (e.g., "public.order_status") to unqualified names
// (e.g., "order_status") when the type is in the same schema as the function/procedure.
//
// Cross-schema type references are preserved (e.g., "utils.priority_level" stays qualified
// when the function is in the "public" schema).
//
// This ensures consistent comparison between database inspection (which may return qualified
// names) and source SQL (which typically uses unqualified names for same-schema types).
func (i *Inspector) stripSameSchemaPrefix(typeName, routineSchema string) string {
	if typeName == "" || routineSchema == "" {
		return typeName
	}

	// Remove quotes from schema name for comparison
	unquotedSchema := routineSchema
	if strings.HasPrefix(routineSchema, `"`) && strings.HasSuffix(routineSchema, `"`) {
		unquotedSchema = routineSchema[1 : len(routineSchema)-1]
	}

	// Handle quoted schema prefix: "schema".typename
	quotedPrefix := fmt.Sprintf(`"%s".`, unquotedSchema)
	if strings.HasPrefix(typeName, quotedPrefix) {
		return typeName[len(quotedPrefix):]
	}

	// Handle unquoted schema prefix: schema.typename
	unquotedPrefix := unquotedSchema + "."
	if strings.HasPrefix(typeName, unquotedPrefix) {
		return typeName[len(unquotedPrefix):]
	}

	// No matching prefix - return as-is (could be cross-schema type or already unqualified)
	return typeName
}

// oidToTypeName maps PostgreSQL type OIDs to standard SQL type names.
// Reference: https://github.com/postgres/postgres/blob/master/src/include/catalog/pg_type.dat
var oidToTypeName = map[int64]string{
	16:   "boolean",
	20:   "bigint",
	21:   "smallint",
	23:   "integer",
	25:   "text",
	1043: "character varying",
	1082: "date",
	1114: "timestamp without time zone", // Will be normalized later
	1184: "timestamp with time zone",
	1700: "numeric",
	2950: "uuid",
}

// lookupTypeNameFromOID converts PostgreSQL type OID to type name
func (i *Inspector) lookupTypeNameFromOID(oid int64) string {
	if typeName, exists := oidToTypeName[oid]; exists {
		return typeName
	}

	// For unknown OIDs, return a placeholder
	// In a real implementation, this could query pg_type
	return fmt.Sprintf("oid_%d", oid)
}

func (i *Inspector) buildProcedures(ctx context.Context, schema *IR, targetSchema string) error {
	procedures, err := i.queries.GetProceduresForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, proc := range procedures {
		schemaName := i.safeInterfaceToString(proc.RoutineSchema)
		procedureName := i.safeInterfaceToString(proc.RoutineName)
		comment := ""
		if proc.ProcedureComment.Valid {
			comment = proc.ProcedureComment.String
		}
		signature := i.safeInterfaceToString(proc.ProcedureSignature)

		// Check if procedure should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreProcedure(procedureName) {
			continue
		}

		// Get procedure definition from pg_get_functiondef
		definition := i.safeInterfaceToString(proc.RoutineDefinition)

		dbSchema := schema.getOrCreateSchema(schemaName)

		// Parse parameters from signature (same approach as functions)
		parameters := i.parseParametersFromSignature(signature, schemaName)

		procedure := &Procedure{
			Schema:     schemaName,
			Name:       procedureName,
			Definition: definition,
			Language:   i.safeInterfaceToString(proc.ExternalLanguage),
			Comment:    comment,
			Parameters: parameters,
		}

		// Use name(arguments) as key to support procedure overloading
		// This allows multiple procedures with the same name but different signatures
		procedureKey := procedureName + "(" + procedure.GetArguments() + ")"
		dbSchema.SetProcedure(procedureKey, procedure)
	}

	return nil
}

func (i *Inspector) buildAggregates(ctx context.Context, schema *IR, targetSchema string) error {
	aggregates, err := i.queries.GetAggregatesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, agg := range aggregates {
		schemaName := agg.AggregateSchema
		aggregateName := agg.AggregateName
		comment := ""
		if agg.AggregateComment.Valid {
			comment = agg.AggregateComment.String
		}
		returnType := i.safeInterfaceToString(agg.AggregateReturnType)
		transitionFunction := i.safeInterfaceToString(agg.TransitionFunction)
		transitionFunctionSchema := i.safeInterfaceToString(agg.TransitionFunctionSchema)
		stateType := i.safeInterfaceToString(agg.StateType)
		initialCondition := i.safeInterfaceToString(agg.InitialCondition)
		finalFunction := i.safeInterfaceToString(agg.FinalFunction)
		finalFunctionSchema := i.safeInterfaceToString(agg.FinalFunctionSchema)

		dbSchema := schema.getOrCreateSchema(schemaName)

		aggregate := &Aggregate{
			Schema:                   schemaName,
			Name:                     aggregateName,
			ReturnType:               returnType,
			TransitionFunction:       transitionFunction,
			TransitionFunctionSchema: transitionFunctionSchema,
			StateType:                stateType,
			InitialCondition:         initialCondition,
			FinalFunction:            finalFunction,
			FinalFunctionSchema:      finalFunctionSchema,
			Comment:                  comment,
		}

		dbSchema.SetAggregate(aggregateName, aggregate)
	}

	return nil
}

func (i *Inspector) buildViews(ctx context.Context, schema *IR, targetSchema string) error {
	views, err := i.queries.GetViewsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	for _, view := range views {
		schemaName := view.TableSchema
		viewName := view.TableName
		comment := ""
		if view.ViewComment.Valid {
			comment = view.ViewComment.String
		}

		// Check if view should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreView(viewName) {
			continue
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		var definition string
		if view.ViewDefinition.Valid {
			definition = view.ViewDefinition.String
			// Strip trailing semicolon from pg_get_viewdef() output for consistency with parser
			definition = strings.TrimSuffix(definition, ";")
		}

		// Fetch view column names from pg_attribute (ordered by attnum)
		columns, err := i.getViewColumns(ctx, schemaName, viewName)
		if err != nil {
			return fmt.Errorf("failed to get columns for view %s.%s: %w", schemaName, viewName, err)
		}

		// Copy and sort reloptions for deterministic comparison and output
		options := append([]string(nil), view.Reloptions...)
		sort.Strings(options)

		v := &View{
			Schema:       schemaName,
			Name:         viewName,
			Definition:   definition,
			Columns:      columns,
			Options:      options,
			Comment:      comment,
			Materialized: view.IsMaterialized.Valid && view.IsMaterialized.Bool,
		}

		dbSchema.SetView(viewName, v)
	}

	return nil
}

// getViewColumns returns the ordered list of column names for a view or materialized view.
// Uses pg_attribute to get column names ordered by their position (attnum).
func (i *Inspector) getViewColumns(ctx context.Context, schemaName, viewName string) ([]string, error) {
	query := `
		SELECT a.attname
		FROM pg_attribute a
		JOIN pg_class c ON a.attrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		WHERE n.nspname = $1
		  AND c.relname = $2
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		ORDER BY a.attnum`

	rows, err := i.db.QueryContext(ctx, query, schemaName, viewName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var colName string
		if err := rows.Scan(&colName); err != nil {
			return nil, err
		}
		columns = append(columns, colName)
	}
	return columns, rows.Err()
}

// extractWhenClauseFromTriggerDef extracts the WHEN clause from a trigger definition
// returned by pg_get_triggerdef(). The format is:
// "CREATE TRIGGER name ... WHEN (condition) EXECUTE FUNCTION ..."
func extractWhenClauseFromTriggerDef(triggerDef string) string {
	// Find "WHEN (" in the definition
	whenIdx := strings.Index(strings.ToUpper(triggerDef), "WHEN (")
	if whenIdx == -1 {
		return ""
	}

	// Start after "WHEN "
	start := whenIdx + 5 // len("WHEN ")

	// Find the matching closing parenthesis before " EXECUTE"
	// We need to count parentheses to handle nested expressions
	parenCount := 0
	inParen := false
	end := -1

	for i := start; i < len(triggerDef); i++ {
		switch triggerDef[i] {
		case '(':
			parenCount++
			inParen = true
		case ')':
			parenCount--
			if parenCount == 0 && inParen {
				end = i + 1
				break
			}
		}
		if end != -1 {
			break
		}
	}

	if end == -1 {
		return ""
	}

	return strings.TrimSpace(triggerDef[start:end])
}

// extractUpdateColumnsFromTriggerDef extracts column names from UPDATE OF col1, col2 in a trigger definition
// returned by pg_get_triggerdef(). The format is:
// "CREATE TRIGGER name BEFORE INSERT OR UPDATE OF col1, col2 ON table ..."
func extractUpdateColumnsFromTriggerDef(triggerDef string) []string {
	upper := strings.ToUpper(triggerDef)
	updateOfIdx := strings.Index(upper, "UPDATE OF ")
	if updateOfIdx == -1 {
		return nil
	}

	// Start after "UPDATE OF "
	start := updateOfIdx + len("UPDATE OF ")

	// Find " ON " which terminates the event/column list
	onIdx := strings.Index(upper[start:], " ON ")
	if onIdx == -1 {
		return nil
	}

	// Extract the column list substring
	colListStr := strings.TrimSpace(triggerDef[start : start+onIdx])

	// Handle potential " OR " separating additional events after the columns
	// e.g., "UPDATE OF col1, col2 OR DELETE ON ..."
	// We need to check if there's an OR followed by another event keyword
	eventKeywords := []string{"INSERT", "UPDATE", "DELETE", "TRUNCATE"}
	parts := strings.Split(colListStr, " OR ")
	// Only keep the first part (column list); the rest would be other events
	if len(parts) > 1 {
		// Check if subsequent parts are event keywords
		for i := 1; i < len(parts); i++ {
			trimmed := strings.TrimSpace(strings.ToUpper(parts[i]))
			isEvent := false
			for _, kw := range eventKeywords {
				if trimmed == kw || strings.HasPrefix(trimmed, kw+" ") {
					isEvent = true
					break
				}
			}
			if isEvent {
				colListStr = strings.TrimSpace(parts[0])
				break
			}
		}
	}

	// Split by comma and trim each column name
	rawCols := strings.Split(colListStr, ",")
	var columns []string
	for _, col := range rawCols {
		col = strings.TrimSpace(col)
		if col != "" {
			columns = append(columns, col)
		}
	}

	return columns
}

// extractFunctionCallFromTriggerDef extracts the function call (with arguments) from a trigger definition
// returned by pg_get_triggerdef(). The format is:
// "... EXECUTE FUNCTION function_name(arg1, arg2)"
func extractFunctionCallFromTriggerDef(triggerDef string) string {
	// Find "EXECUTE FUNCTION" or "EXECUTE PROCEDURE" in the definition
	executeIdx := strings.Index(strings.ToUpper(triggerDef), "EXECUTE FUNCTION ")
	if executeIdx == -1 {
		executeIdx = strings.Index(strings.ToUpper(triggerDef), "EXECUTE PROCEDURE ")
		if executeIdx == -1 {
			return ""
		}
	}

	// Start after "EXECUTE FUNCTION " or "EXECUTE PROCEDURE "
	start := strings.Index(triggerDef[executeIdx:], " ") + executeIdx + 1 // Skip "EXECUTE"
	start = strings.Index(triggerDef[start:], " ") + start + 1            // Skip "FUNCTION"/"PROCEDURE"

	// The function call extends to the end of the definition (or a semicolon if present)
	end := len(triggerDef)
	if semiIdx := strings.Index(triggerDef[start:], ";"); semiIdx != -1 {
		end = start + semiIdx
	}

	return strings.TrimSpace(triggerDef[start:end])
}

func (i *Inspector) buildTriggers(ctx context.Context, schema *IR, targetSchema string) error {
	triggers, err := i.queries.GetTriggersForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Process each trigger from pg_trigger catalog
	for _, triggerRow := range triggers {
		tableName := triggerRow.EventObjectTable
		schemaName := triggerRow.TriggerSchema
		triggerName := triggerRow.TriggerName

		// Find where to store this trigger: table, view, or ignored external table
		targetDBSchema := schema.getOrCreateSchema(schemaName)
		var triggerMap map[string]*Trigger

		if table, exists := targetDBSchema.Tables[tableName]; exists {
			triggerMap = table.Triggers
		} else if view, exists := targetDBSchema.Views[tableName]; exists {
			// Trigger on a view (e.g., INSTEAD OF triggers)
			if view.Triggers == nil {
				view.Triggers = make(map[string]*Trigger)
			}
			triggerMap = view.Triggers
		} else if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreTable(tableName) {
			// Check if the table is ignored - if so, create external table stub to hold trigger
			// This allows users to manage triggers on externally-managed tables
			table := &Table{
				Schema:      schemaName,
				Name:        tableName,
				Type:        TableTypeBase,
				IsExternal:  true,
				Columns:     []*Column{},
				Constraints: make(map[string]*Constraint),
				Indexes:     make(map[string]*Index),
				Triggers:    make(map[string]*Trigger),
				Policies:    make(map[string]*RLSPolicy),
			}
			targetDBSchema.Tables[tableName] = table
			triggerMap = table.Triggers
		} else {
			// Relation doesn't exist and isn't ignored - skip this trigger
			continue
		}

		// Decode trigger type bitmask to extract timing, events, and level
		timing := i.decodeTriggerTiming(triggerRow.TriggerType)
		events := i.decodeTriggerEvents(triggerRow.TriggerType)
		level := i.decodeTriggerLevel(triggerRow.TriggerType)

		// Extract function call with arguments from trigger definition
		functionCall := ""
		if triggerRow.TriggerDefinition.Valid {
			functionCall = extractFunctionCallFromTriggerDef(triggerRow.TriggerDefinition.String)
		}
		// Fallback to basic function name if extraction failed
		if functionCall == "" {
			functionCall = triggerRow.FunctionName + "()"
			if triggerRow.FunctionSchema != schemaName {
				// Include schema qualifier if different from trigger's schema
				functionCall = triggerRow.FunctionSchema + "." + functionCall
			}
		}

		// Extract WHEN clause from trigger definition
		condition := ""
		if triggerRow.TriggerDefinition.Valid {
			condition = extractWhenClauseFromTriggerDef(triggerRow.TriggerDefinition.String)
		}

		// Extract UPDATE OF columns from trigger definition (only for triggers with UPDATE events)
		var updateColumns []string
		if triggerRow.TriggerDefinition.Valid && triggerRow.TriggerType&triggerTypeUpdate != 0 {
			updateColumns = extractUpdateColumnsFromTriggerDef(triggerRow.TriggerDefinition.String)
		}

		// Extract transition table names
		oldTable := ""
		if triggerRow.OldTable.Valid {
			oldTable = triggerRow.OldTable.String
		}
		newTable := ""
		if triggerRow.NewTable.Valid {
			newTable = triggerRow.NewTable.String
		}

		// Extract comment
		comment := ""
		if triggerRow.TriggerComment.Valid {
			comment = triggerRow.TriggerComment.String
		}

		// Determine if this is a constraint trigger
		oid, ok := triggerRow.TriggerConstraintOid.(int64)
		isConstraint := ok && oid != 0
		deferrable := triggerRow.TriggerDeferrable
		initDeferred := triggerRow.TriggerInitdeferred

		// Create trigger object
		trigger := &Trigger{
			Schema:            schemaName,
			Name:              triggerName,
			Table:             tableName,
			Timing:            timing,
			Events:            events,
			UpdateColumns:     updateColumns,
			Level:             level,
			Function:          functionCall,
			Condition:         condition,
			OldTable:          oldTable,
			NewTable:          newTable,
			IsConstraint:      isConstraint,
			Deferrable:        deferrable,
			InitiallyDeferred: initDeferred,
			Comment:           comment,
		}

		// Add trigger to the appropriate map
		triggerMap[triggerName] = trigger
	}

	return nil
}

// decodeTriggerTiming decodes trigger timing from pg_trigger.tgtype bitmask
func (i *Inspector) decodeTriggerTiming(tgtype int16) TriggerTiming {
	if tgtype&triggerTypeInstead != 0 {
		return TriggerTimingInsteadOf
	}
	if tgtype&triggerTypeBefore != 0 {
		return TriggerTimingBefore
	}
	// If neither BEFORE nor INSTEAD, then it's AFTER
	return TriggerTimingAfter
}

// decodeTriggerEvents decodes trigger events from pg_trigger.tgtype bitmask
func (i *Inspector) decodeTriggerEvents(tgtype int16) []TriggerEvent {
	var events []TriggerEvent

	if tgtype&triggerTypeInsert != 0 {
		events = append(events, TriggerEventInsert)
	}
	if tgtype&triggerTypeUpdate != 0 {
		events = append(events, TriggerEventUpdate)
	}
	if tgtype&triggerTypeDelete != 0 {
		events = append(events, TriggerEventDelete)
	}
	if tgtype&triggerTypeTruncate != 0 {
		events = append(events, TriggerEventTruncate)
	}

	return events
}

// decodeTriggerLevel decodes trigger level from pg_trigger.tgtype bitmask
func (i *Inspector) decodeTriggerLevel(tgtype int16) TriggerLevel {
	if tgtype&triggerTypeRow != 0 {
		return TriggerLevelRow
	}
	return TriggerLevelStatement
}

func (i *Inspector) buildRLSPolicies(ctx context.Context, schema *IR, targetSchema string) error {
	// Get RLS enabled tables for the target schema
	rlsTables, err := i.queries.GetRLSTablesForSchema(ctx, targetSchema)
	if err != nil {
		return err
	}

	// Mark tables as RLS enabled/forced
	for _, rlsTable := range rlsTables {
		dbSchema := schema.getOrCreateSchema(rlsTable.Schemaname)
		if table, exists := dbSchema.Tables[rlsTable.Tablename]; exists {
			// Query filters by rowsecurity = true, so this is always true
			table.RLSEnabled = true
			table.RLSForced = rlsTable.Rowforced
		}
	}

	// Get RLS policies for the target schema
	policies, err := i.queries.GetRLSPoliciesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Process policies
	for _, policyRow := range policies {
		schemaName := ""
		if policyRow.Schemaname.Valid {
			schemaName = policyRow.Schemaname.String
		}
		tableName := ""
		if policyRow.Tablename.Valid {
			tableName = policyRow.Tablename.String
		}
		policyName := ""
		if policyRow.Policyname.Valid {
			policyName = policyRow.Policyname.String
		}

		var pCommand PolicyCommand
		if policyRow.Cmd.Valid {
			switch policyRow.Cmd.String {
			case "SELECT":
				pCommand = PolicyCommandSelect
			case "INSERT":
				pCommand = PolicyCommandInsert
			case "UPDATE":
				pCommand = PolicyCommandUpdate
			case "DELETE":
				pCommand = PolicyCommandDelete
			case "ALL":
				pCommand = PolicyCommandAll
			default:
				pCommand = PolicyCommandAll
			}
		} else {
			pCommand = PolicyCommandAll
		}

		// Determine if policy is permissive
		permissive := true // Default
		if policyRow.Permissive.Valid {
			permissive = policyRow.Permissive.String == "PERMISSIVE"
		}

		policy := &RLSPolicy{
			Schema:     schemaName,
			Table:      tableName,
			Name:       policyName,
			Command:    pCommand,
			Permissive: permissive,
			Roles:      policyRow.Roles,
		}

		if policyRow.Qual.Valid {
			policy.Using = policyRow.Qual.String
		}

		if policyRow.WithCheck.Valid {
			policy.WithCheck = policyRow.WithCheck.String
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		if table, exists := dbSchema.Tables[tableName]; exists {
			table.Policies[policyName] = policy
		}
	}

	return nil
}

func (i *Inspector) buildTypes(ctx context.Context, schema *IR, targetSchema string) error {
	types, err := i.queries.GetTypesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Get domains
	domains, err := i.queries.GetDomainsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Get domain constraints
	domainConstraints, err := i.queries.GetDomainConstraintsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Get enum values for ENUM types
	enumValues, err := i.queries.GetEnumValuesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Get columns for composite types
	compositeColumns, err := i.queries.GetCompositeTypeColumnsForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	// Create maps for efficient lookup
	enumValuesMap := make(map[string][]string)
	compositeColumnsMap := make(map[string][]*TypeColumn)
	domainConstraintsMap := make(map[string][]*DomainConstraint)

	// Process enum values
	for _, enumVal := range enumValues {
		key := fmt.Sprintf("%s.%s", enumVal.TypeSchema, enumVal.TypeName)
		enumValuesMap[key] = append(enumValuesMap[key], enumVal.EnumValue)
	}

	// Process composite columns
	for _, col := range compositeColumns {
		key := fmt.Sprintf("%s.%s", col.TypeSchema, col.TypeName)
		position := i.safeInterfaceToInt(col.ColumnPosition, 0)

		dataType := ""
		if col.ColumnType.Valid {
			dataType = col.ColumnType.String
		}

		typeCol := &TypeColumn{
			Name:     col.ColumnName,
			DataType: dataType,
			Position: position,
		}

		compositeColumnsMap[key] = append(compositeColumnsMap[key], typeCol)
	}

	// Process domain constraints
	for _, constraint := range domainConstraints {
		key := fmt.Sprintf("%s.%s", i.safeInterfaceToString(constraint.DomainSchema), i.safeInterfaceToString(constraint.DomainName))
		constraintName := i.safeInterfaceToString(constraint.ConstraintName)
		constraintDef := i.safeInterfaceToString(constraint.ConstraintDefinition)

		// Skip NOT NULL constraints as they are already captured in the NotNull boolean field
		if constraintDef == "NOT NULL" {
			continue
		}

		domainConstraint := &DomainConstraint{
			Name:       constraintName,
			Definition: constraintDef,
		}

		domainConstraintsMap[key] = append(domainConstraintsMap[key], domainConstraint)
	}

	// Create types
	for _, t := range types {
		schemaName := t.TypeSchema
		typeName := t.TypeName
		typeKindStr := ""
		if t.TypeKind.Valid {
			typeKindStr = t.TypeKind.String
		}
		typeKind := TypeKind(typeKindStr)
		comment := ""
		if t.TypeComment.Valid {
			comment = t.TypeComment.String
		}

		// Check if type should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreType(typeName) {
			continue
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		customType := &Type{
			Schema:  schemaName,
			Name:    typeName,
			Kind:    typeKind,
			Comment: comment,
		}

		key := fmt.Sprintf("%s.%s", schemaName, typeName)

		switch typeKind {
		case TypeKindEnum:
			customType.EnumValues = enumValuesMap[key]
		case TypeKindComposite:
			customType.Columns = compositeColumnsMap[key]
		}

		dbSchema.SetType(typeName, customType)
	}

	// Create domains
	for _, d := range domains {
		schemaName := i.safeInterfaceToString(d.DomainSchema)
		domainName := i.safeInterfaceToString(d.DomainName)
		baseType := i.safeInterfaceToString(d.BaseType)
		notNull := i.safeInterfaceToBool(d.NotNull, false)
		defaultValue := i.safeInterfaceToString(d.DefaultValue)
		comment := ""
		if d.DomainComment.Valid {
			comment = d.DomainComment.String
		}

		// Check if domain (type) should be ignored
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreType(domainName) {
			continue
		}

		dbSchema := schema.getOrCreateSchema(schemaName)

		key := fmt.Sprintf("%s.%s", schemaName, domainName)
		constraints := domainConstraintsMap[key]

		domainType := &Type{
			Schema:      schemaName,
			Name:        domainName,
			Kind:        TypeKindDomain,
			Comment:     comment,
			BaseType:    baseType,
			NotNull:     notNull,
			Default:     defaultValue,
			Constraints: constraints,
		}

		dbSchema.SetType(domainName, domainType)
	}

	return nil
}

// Helper methods

func (i *Inspector) getConstraintColumnPosition(ctx context.Context, schemaName, constraintName, columnName string) int {
	query := `
		SELECT kcu.ordinal_position
		FROM information_schema.key_column_usage kcu
		WHERE kcu.table_schema = $1
		  AND kcu.constraint_name = $2
		  AND kcu.column_name = $3`

	var position int
	err := i.db.QueryRowContext(ctx, query, schemaName, constraintName, columnName).Scan(&position)
	if err != nil {
		return 0 // Default position if query fails
	}

	return position
}

// pgCatalogTriggerFunctions contains the hard-coded list of built-in PostgreSQL trigger functions
// that exist in the pg_catalog schema. These functions should always be schema-qualified
// to maintain consistency between parser and inspector outputs.
var pgCatalogTriggerFunctions = map[string]bool{
	"suppress_redundant_updates_trigger": true,
	"tsvector_update_trigger":            true,
	"tsvector_update_trigger_column":     true,
}

func (i *Inspector) extractFunctionFromStatement(statement string) string {
	// Extract complete function call from "EXECUTE FUNCTION function_name(...)"
	if strings.Contains(statement, "EXECUTE FUNCTION ") {
		parts := strings.Split(statement, "EXECUTE FUNCTION ")
		if len(parts) > 1 {
			funcPart := strings.TrimSpace(parts[1])

			// Check if this is a pg_catalog built-in function that needs schema qualification
			if needsSchemaQualification := i.shouldAddPgCatalogPrefix(funcPart); needsSchemaQualification {
				// Add pg_catalog prefix if it's missing
				if !strings.HasPrefix(funcPart, "pg_catalog.") {
					funcPart = "pg_catalog." + funcPart
				}
			}

			// Return the complete function call including parameters
			return funcPart
		}
	}
	return statement
}

// shouldAddPgCatalogPrefix determines if a function name should be prefixed with pg_catalog.
// It extracts the base function name and checks if it's in the list of built-in trigger functions.
func (i *Inspector) shouldAddPgCatalogPrefix(funcCall string) bool {
	// Extract just the function name (before the opening parenthesis)
	funcName := funcCall
	if parenIndex := strings.Index(funcCall, "("); parenIndex != -1 {
		funcName = funcCall[:parenIndex]
	}

	// Remove any existing schema qualification to get the base name
	if dotIndex := strings.LastIndex(funcName, "."); dotIndex != -1 {
		funcName = funcName[dotIndex+1:]
	}

	// Check if it's a pg_catalog built-in function
	return pgCatalogTriggerFunctions[funcName]
}

// validateSchemaExists checks if a schema exists in the database
func (i *Inspector) validateSchemaExists(ctx context.Context, schemaName string) error {
	query := `
		SELECT 1 
		FROM information_schema.schemata 
		WHERE schema_name = $1
		  AND schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
		  AND schema_name NOT LIKE 'pg_temp_%'
		  AND schema_name NOT LIKE 'pg_toast_temp_%'`

	var exists int
	err := i.db.QueryRowContext(ctx, query, schemaName).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("schema '%s' does not exist in the database", schemaName)
	}
	if err != nil {
		return fmt.Errorf("failed to check if schema '%s' exists: %w", schemaName, err)
	}

	return nil
}

// buildPrivileges retrieves explicit privilege grants for objects in the schema
func (i *Inspector) buildPrivileges(ctx context.Context, schema *IR, targetSchema string) error {
	rows, err := i.queries.GetPrivilegesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to query privileges: %w", err)
	}

	// Group privileges by (object_type, object_name, grantee, is_grantable)
	type privKey struct {
		ObjectType      string
		ObjectName      string
		Grantee         string
		WithGrantOption bool
	}

	grouped := make(map[privKey][]string)

	for _, row := range rows {
		objectName := row.ObjectName.String
		objectType := row.ObjectType.String
		privilegeType := row.PrivilegeType.String
		owner := row.Owner.String
		isGrantable := row.IsGrantable.Valid && row.IsGrantable.Bool

		// Determine grantee name from OID
		grantee := "PUBLIC"
		if row.GranteeOid != nil {
			// Handle different possible types for grantee OID
			var granteeOID int64
			switch v := row.GranteeOid.(type) {
			case int64:
				granteeOID = v
			case int32:
				granteeOID = int64(v)
			case int:
				granteeOID = int64(v)
			}
			if granteeOID != 0 {
				// Look up role name
				var roleName string
				err := i.db.QueryRowContext(ctx, "SELECT rolname FROM pg_roles WHERE oid = $1", granteeOID).Scan(&roleName)
				if err != nil {
					if err == sql.ErrNoRows {
						continue // Role no longer exists, skip this privilege
					}
					return fmt.Errorf("failed to lookup role name for OID %d: %w", granteeOID, err)
				}
				grantee = roleName
			}
		}

		// Skip owner grants (owners implicitly have all privileges)
		if grantee == owner {
			continue
		}

		// Skip privileges for ignored grantees
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnorePrivilege(grantee) {
			continue
		}

		// Check for default PUBLIC grants that should be excluded
		if grantee == "PUBLIC" {
			if (objectType == "FUNCTION" || objectType == "PROCEDURE") && privilegeType == "EXECUTE" {
				// Default PUBLIC EXECUTE on functions/procedures - skip
				continue
			}
			if objectType == "TYPE" && privilegeType == "USAGE" {
				// Default PUBLIC USAGE on types - skip
				continue
			}
		}

		key := privKey{
			ObjectType:      objectType,
			ObjectName:      objectName,
			Grantee:         grantee,
			WithGrantOption: isGrantable,
		}

		grouped[key] = append(grouped[key], privilegeType)
	}

	// Convert to Privilege structs
	var privileges []*Privilege
	for key, privs := range grouped {
		// Sort privileges for deterministic output
		sort.Strings(privs)

		var objType PrivilegeObjectType
		switch key.ObjectType {
		case "TABLE":
			objType = PrivilegeObjectTypeTable
		case "VIEW":
			objType = PrivilegeObjectTypeView
		case "SEQUENCE":
			objType = PrivilegeObjectTypeSequence
		case "FUNCTION":
			objType = PrivilegeObjectTypeFunction
		case "PROCEDURE":
			objType = PrivilegeObjectTypeProcedure
		case "TYPE":
			objType = PrivilegeObjectTypeType
		default:
			continue
		}

		p := &Privilege{
			ObjectType:      objType,
			ObjectName:      key.ObjectName,
			Grantee:         key.Grantee,
			Privileges:      privs,
			WithGrantOption: key.WithGrantOption,
		}
		privileges = append(privileges, p)
	}

	// Sort for deterministic output
	sort.Slice(privileges, func(i, j int) bool {
		if privileges[i].ObjectType != privileges[j].ObjectType {
			return privileges[i].ObjectType < privileges[j].ObjectType
		}
		if privileges[i].ObjectName != privileges[j].ObjectName {
			return privileges[i].ObjectName < privileges[j].ObjectName
		}
		return privileges[i].Grantee < privileges[j].Grantee
	})

	// Now check for revoked default privileges (functions/procedures/types where PUBLIC has no access)
	revokedDefaults, err := i.buildRevokedDefaultPrivileges(ctx, targetSchema)
	if err != nil {
		return fmt.Errorf("failed to build revoked default privileges: %w", err)
	}

	// Assign to schema
	s, ok := schema.GetSchema(targetSchema)
	if ok {
		s.Privileges = privileges
		s.RevokedDefaultPrivileges = revokedDefaults
	}

	return nil
}

// buildRevokedDefaultPrivileges finds objects where default PUBLIC grants have been explicitly revoked
func (i *Inspector) buildRevokedDefaultPrivileges(ctx context.Context, targetSchema string) ([]*RevokedDefaultPrivilege, error) {
	rows, err := i.queries.GetRevokedDefaultPrivilegesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to query revoked default privileges: %w", err)
	}

	var revoked []*RevokedDefaultPrivilege
	for _, row := range rows {
		objectName := row.ObjectName.String
		objectType := row.ObjectType.String

		var objType PrivilegeObjectType
		var privs []string
		switch objectType {
		case "FUNCTION", "PROCEDURE":
			objType = PrivilegeObjectType(objectType)
			privs = []string{"EXECUTE"}
		case "TYPE":
			objType = PrivilegeObjectTypeType
			privs = []string{"USAGE"}
		default:
			continue
		}

		r := &RevokedDefaultPrivilege{
			ObjectType: objType,
			ObjectName: objectName,
			Privileges: privs,
		}
		revoked = append(revoked, r)
	}

	return revoked, nil
}

// buildDefaultPrivileges retrieves default privileges for the schema
func (i *Inspector) buildDefaultPrivileges(ctx context.Context, schema *IR, targetSchema string) error {
	privileges, err := i.queries.GetDefaultPrivilegesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return err
	}

	if len(privileges) == 0 {
		return nil
	}

	// Group privileges by (owner_role, object_type, grantee, is_grantable)
	type privKey struct {
		OwnerRole       string
		ObjectType      string
		Grantee         string
		WithGrantOption bool
	}

	grouped := make(map[privKey][]string)
	for _, p := range privileges {
		if !p.OwnerRole.Valid || !p.ObjectType.Valid || !p.Grantee.Valid || !p.PrivilegeType.Valid {
			continue
		}

		// Skip default privileges for ignored grantees
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnoreDefaultPrivilege(p.Grantee.String) {
			continue
		}

		key := privKey{
			OwnerRole:       p.OwnerRole.String,
			ObjectType:      p.ObjectType.String,
			Grantee:         p.Grantee.String,
			WithGrantOption: p.IsGrantable.Valid && p.IsGrantable.Bool,
		}

		grouped[key] = append(grouped[key], p.PrivilegeType.String)
	}

	// Convert to DefaultPrivilege structs
	var defaultPrivileges []*DefaultPrivilege
	for key, privs := range grouped {
		// Sort privileges for deterministic IR output
		sort.Strings(privs)
		dp := &DefaultPrivilege{
			OwnerRole:       key.OwnerRole,
			ObjectType:      DefaultPrivilegeObjectType(key.ObjectType),
			Grantee:         key.Grantee,
			Privileges:      privs,
			WithGrantOption: key.WithGrantOption,
		}
		defaultPrivileges = append(defaultPrivileges, dp)
	}

	// Sort for deterministic output (by owner_role, then object_type, then grantee)
	sort.Slice(defaultPrivileges, func(i, j int) bool {
		if defaultPrivileges[i].OwnerRole != defaultPrivileges[j].OwnerRole {
			return defaultPrivileges[i].OwnerRole < defaultPrivileges[j].OwnerRole
		}
		if defaultPrivileges[i].ObjectType != defaultPrivileges[j].ObjectType {
			return defaultPrivileges[i].ObjectType < defaultPrivileges[j].ObjectType
		}
		return defaultPrivileges[i].Grantee < defaultPrivileges[j].Grantee
	})

	// Assign to schema
	s, ok := schema.GetSchema(targetSchema)
	if ok {
		s.DefaultPrivileges = defaultPrivileges
	}

	return nil
}

// Helper functions for safe type conversion from interface{}

func (i *Inspector) safeInterfaceToString(val interface{}) string {
	if val == nil {
		return ""
	}
	if sqlVal, ok := val.(sql.NullString); ok {
		if sqlVal.Valid {
			return sqlVal.String
		}
		return ""
	}
	return fmt.Sprintf("%s", val)
}

func (i *Inspector) safeInterfaceToInt(val interface{}, defaultVal int) int {
	if val == nil {
		return defaultVal
	}
	if sqlVal, ok := val.(sql.NullInt64); ok {
		if sqlVal.Valid {
			return int(sqlVal.Int64)
		}
		return defaultVal
	}
	if intVal, ok := val.(int64); ok {
		return int(intVal)
	}
	if intVal, ok := val.(int32); ok {
		return int(intVal)
	}
	if intVal, ok := val.(int); ok {
		return intVal
	}
	return defaultVal
}

func (i *Inspector) safeInterfaceToInt64(val interface{}, defaultVal int64) int64 {
	if val == nil {
		return defaultVal
	}
	if sqlVal, ok := val.(sql.NullInt64); ok {
		if sqlVal.Valid {
			return sqlVal.Int64
		}
		return defaultVal
	}
	if intVal, ok := val.(int64); ok {
		return intVal
	}
	if intVal, ok := val.(int32); ok {
		return int64(intVal)
	}
	if intVal, ok := val.(int); ok {
		return int64(intVal)
	}
	// Handle string types (information_schema.sequences returns numeric values as strings)
	if strVal, ok := val.(string); ok {
		if parsedVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
			return parsedVal
		}
	}
	return defaultVal
}

func (i *Inspector) safeInterfaceToBool(val interface{}, defaultVal bool) bool {
	if val == nil {
		return defaultVal
	}
	if sqlVal, ok := val.(sql.NullBool); ok {
		if sqlVal.Valid {
			return sqlVal.Bool
		}
		return defaultVal
	}
	if boolVal, ok := val.(bool); ok {
		return boolVal
	}
	if strVal := i.safeInterfaceToString(val); strVal != "" {
		return strVal == "YES" || strVal == "true" || strVal == "t"
	}
	return defaultVal
}

// getSequenceMaxValueForType returns the default maximum value for a sequence based on its data type
func getSequenceMaxValueForType(dataType string) int64 {
	switch dataType {
	case "smallint":
		return smallintMaxValue
	case "integer":
		return integerMaxValue
	default:
		// bigint is the default for sequences when no type is specified
		return bigintMaxValue
	}
}

// buildColumnPrivileges retrieves column-level privilege grants for the schema
func (i *Inspector) buildColumnPrivileges(ctx context.Context, schema *IR, targetSchema string) error {
	rows, err := i.queries.GetColumnPrivilegesForSchema(ctx, sql.NullString{String: targetSchema, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to query column privileges: %w", err)
	}

	// Group by (table, grantee, privilege_type, is_grantable) to collect columns
	// Then further group by column set to combine privilege types
	type colPrivKey struct {
		TableName       string
		Grantee         string
		WithGrantOption bool
	}

	// First, collect all column->privileges for each (table, grantee, grant_option)
	type columnPrivs struct {
		Columns    map[string][]string // column_name -> []privilege_type
		Privileges map[string]bool     // unique privilege types
	}
	grouped := make(map[colPrivKey]*columnPrivs)

	for _, row := range rows {
		tableName := row.TableName
		columnName := row.ColumnName
		privilegeType := row.PrivilegeType.String
		isGrantable := row.IsGrantable.Valid && row.IsGrantable.Bool

		// Determine grantee name from OID
		grantee := "PUBLIC"
		if row.GranteeOid != nil {
			var granteeOID int64
			switch v := row.GranteeOid.(type) {
			case int64:
				granteeOID = v
			case int32:
				granteeOID = int64(v)
			case int:
				granteeOID = int64(v)
			}
			if granteeOID != 0 {
				var roleName string
				err := i.db.QueryRowContext(ctx, "SELECT rolname FROM pg_roles WHERE oid = $1", granteeOID).Scan(&roleName)
				if err != nil {
					if err == sql.ErrNoRows {
						continue // Role no longer exists, skip this privilege
					}
					return fmt.Errorf("failed to lookup role name for OID %d: %w", granteeOID, err)
				}
				grantee = roleName
			}
		}

		// Skip column privileges for ignored grantees
		if i.ignoreConfig != nil && i.ignoreConfig.ShouldIgnorePrivilege(grantee) {
			continue
		}

		key := colPrivKey{
			TableName:       tableName,
			Grantee:         grantee,
			WithGrantOption: isGrantable,
		}

		if grouped[key] == nil {
			grouped[key] = &columnPrivs{
				Columns:    make(map[string][]string),
				Privileges: make(map[string]bool),
			}
		}

		grouped[key].Columns[columnName] = append(grouped[key].Columns[columnName], privilegeType)
		grouped[key].Privileges[privilegeType] = true
	}

	// Convert to ColumnPrivilege structs
	// Group columns that have the same set of privileges
	var columnPrivileges []*ColumnPrivilege
	for key, cp := range grouped {
		// For each unique set of privileges, collect columns that have exactly those privileges
		// First, normalize: for each column, get its sorted privilege set as a key
		privSetToColumns := make(map[string][]string)
		for col, privs := range cp.Columns {
			sort.Strings(privs)
			privKey := strings.Join(privs, ",")
			privSetToColumns[privKey] = append(privSetToColumns[privKey], col)
		}

		// Create a ColumnPrivilege for each unique privilege set
		for privKey, cols := range privSetToColumns {
			sort.Strings(cols) // Sort columns for deterministic output
			privs := strings.Split(privKey, ",")

			p := &ColumnPrivilege{
				TableName:       key.TableName,
				Columns:         cols,
				Grantee:         key.Grantee,
				Privileges:      privs,
				WithGrantOption: key.WithGrantOption,
			}
			columnPrivileges = append(columnPrivileges, p)
		}
	}

	// Sort for deterministic output
	sort.Slice(columnPrivileges, func(i, j int) bool {
		if columnPrivileges[i].TableName != columnPrivileges[j].TableName {
			return columnPrivileges[i].TableName < columnPrivileges[j].TableName
		}
		// Sort by first column name for determinism
		iCols := strings.Join(columnPrivileges[i].Columns, ",")
		jCols := strings.Join(columnPrivileges[j].Columns, ",")
		if iCols != jCols {
			return iCols < jCols
		}
		return columnPrivileges[i].Grantee < columnPrivileges[j].Grantee
	})

	// Assign to schema
	s, ok := schema.GetSchema(targetSchema)
	if ok {
		s.ColumnPrivileges = columnPrivileges
	}

	return nil
}
