package diff

import (
	"regexp"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// nextvalSequenceNamePattern extracts the bare sequence name from a nextval()
// default expression, e.g. nextval('"questionReference_questionRefId_seq"'::regclass)
// or nextval('domain.some_seq'::regclass) -> "some_seq" (schema prefix and quoting
// stripped, matching how sequence names are keyed elsewhere in this package).
var nextvalSequenceNamePattern = regexp.MustCompile(`nextval\('([^']+)'`)

// nextvalTargetSequenceName returns the bare sequence name a column's nextval()
// default targets, or "" if the column has no such default.
func nextvalTargetSequenceName(column *ir.Column) string {
	if column.DefaultValue == nil {
		return ""
	}
	m := nextvalSequenceNamePattern.FindStringSubmatch(*column.DefaultValue)
	if m == nil {
		return ""
	}
	name := m[1]
	// Strip an optional schema prefix (quoted or bare) before the sequence name.
	if idx := strings.LastIndex(name, "."); idx != -1 {
		name = name[idx+1:]
	}
	return strings.Trim(name, `"`)
}

// topologicallySortTables sorts tables across all schemas in dependency order
// Tables that are referenced by foreign keys will come before the tables that reference them
func topologicallySortTables(tables []*ir.Table) []*ir.Table {
	if len(tables) <= 1 {
		return tables
	}

	// Build maps for efficient lookup
	tableMap := make(map[string]*ir.Table)
	var insertionOrder []string
	for _, table := range tables {
		key := table.Schema + "." + table.Name
		tableMap[key] = table
		insertionOrder = append(insertionOrder, key)
	}

	// Build dependency graph
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for key := range tableMap {
		inDegree[key] = 0
		adjList[key] = []string{}
	}

	// Map bare sequence name -> owning table key, for sequences genuinely OWNED
	// (HasOwnedSequence) by a column in this table set. Used below to order a
	// table whose column merely *references* another table's sequence (e.g. two
	// tables sharing one sequence via an explicit nextval() default, with no
	// formal FK tying them together) after that sequence's real owning table -
	// otherwise CREATE TABLE can run before the sequence the DEFAULT depends on
	// actually exists.
	sequenceOwnerTable := make(map[string]string)
	for key, table := range tableMap {
		for _, col := range table.Columns {
			if col.HasOwnedSequence {
				if seqName := nextvalTargetSequenceName(col); seqName != "" {
					sequenceOwnerTable[seqName] = key
				}
			}
		}
	}

	// Build edges: if tableA has a foreign key to tableB, add edge tableB -> tableA
	// Also: if tableA is a partition child of tableB, add edge tableB -> tableA
	// Also: if tableA has a column whose nextval() default targets a sequence
	// genuinely owned by tableB, add edge tableB -> tableA
	for keyA, tableA := range tableMap {
		// Partition parent → child dependency
		if tableA.PartitionOf != "" {
			parentSchema := tableA.PartitionOfSchema
			if parentSchema == "" {
				parentSchema = tableA.Schema
			}
			keyB := parentSchema + "." + tableA.PartitionOf
			if _, exists := tableMap[keyB]; exists && keyA != keyB {
				adjList[keyB] = append(adjList[keyB], keyA)
				inDegree[keyA]++
			}
		}

		for _, constraint := range tableA.Constraints {
			if constraint.Type == ir.ConstraintTypeForeignKey && constraint.ReferencedTable != "" {
				// Build referenced table key
				referencedSchema := constraint.ReferencedSchema
				if referencedSchema == "" {
					referencedSchema = tableA.Schema // Default to same schema
				}
				keyB := referencedSchema + "." + constraint.ReferencedTable

				// Only add edge if referenced table exists in our set and is different
				if _, exists := tableMap[keyB]; exists && keyA != keyB {
					adjList[keyB] = append(adjList[keyB], keyA)
					inDegree[keyA]++
				}
			}
		}

		// Shared-sequence dependency: a column referencing (but not owning) a
		// sequence must come after the table that genuinely owns it.
		for _, col := range tableA.Columns {
			if col.HasOwnedSequence {
				continue
			}
			seqName := nextvalTargetSequenceName(col)
			if seqName == "" {
				continue
			}
			if keyB, exists := sequenceOwnerTable[seqName]; exists && keyB != keyA {
				adjList[keyB] = append(adjList[keyB], keyA)
				inDegree[keyA]++
			}
		}
	}

	// Kahn's algorithm with deterministic cycle breaking
	var queue []string
	var result []string
	processed := make(map[string]bool, len(tableMap))

	// Seed queue with nodes that have no incoming edges
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	for len(result) < len(tableMap) {
		if len(queue) == 0 {
			// Cycle detected: pick the next unprocessed table using original insertion order
			//
			// CYCLE BREAKING STRATEGY:
			// Setting inDegree[next] = 0 effectively declares "this table has no remaining dependencies"
			// for the purpose of breaking the cycle. This is safe because:
			//
			// 1. The 'processed' map prevents any table from being added to the result twice, even if
			//    its inDegree becomes zero or negative multiple times (see line 92 check).
			//
			// 2. For circular foreign key dependencies (e.g., A↔B), the table creation order doesn't
			//    matter because pgschema follows PostgreSQL's pattern of creating tables first and
			//    adding foreign key constraints afterwards via ALTER TABLE statements.
			//
			// 3. Using insertion order (alphabetical by schema.name) ensures deterministic output
			//    when multiple valid orderings exist.
			//
			// This approach aligns with PostgreSQL's pg_dump, which breaks dependency cycles by
			// separating table creation from constraint creation.
			next := nextInOrder(insertionOrder, processed)
			if next == "" {
				break
			}
			queue = append(queue, next)
			inDegree[next] = 0
		}

		current := queue[0]
		queue = queue[1:]
		if processed[current] {
			continue
		}
		processed[current] = true
		result = append(result, current)

		neighbors := append([]string(nil), adjList[current]...)
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			// Add neighbor to queue if all its dependencies are satisfied.
			// The '!processed[neighbor]' check is critical: it prevents re-adding tables
			// that have already been processed, even if their inDegree becomes <= 0 again
			// due to cycle breaking (where we artificially set inDegree to 0).
			if inDegree[neighbor] <= 0 && !processed[neighbor] {
				queue = append(queue, neighbor)
				sort.Strings(queue)
			}
		}
	}

	// Convert result back to table slice
	sortedTables := make([]*ir.Table, 0, len(result))
	for _, key := range result {
		sortedTables = append(sortedTables, tableMap[key])
	}

	return sortedTables
}

// topologicallySortViews sorts views across all schemas in dependency order
// Views that depend on other views will come after their dependencies
func topologicallySortViews(views []*ir.View) []*ir.View {
	if len(views) <= 1 {
		return views
	}

	// Build maps for efficient lookup
	viewMap := make(map[string]*ir.View)
	var insertionOrder []string
	for _, view := range views {
		key := view.Schema + "." + view.Name
		viewMap[key] = view
		insertionOrder = append(insertionOrder, key)
	}

	// Build dependency graph
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for key := range viewMap {
		inDegree[key] = 0
		adjList[key] = []string{}
	}

	// Build edges: if viewA depends on viewB, add edge viewB -> viewA
	for keyA, viewA := range viewMap {
		for keyB, viewB := range viewMap {
			if keyA != keyB && viewDependsOnView(viewA, viewB.Name) {
				adjList[keyB] = append(adjList[keyB], keyA)
				inDegree[keyA]++
			}
		}
	}

	// Kahn's algorithm with deterministic cycle breaking
	var queue []string
	var result []string
	processed := make(map[string]bool, len(viewMap))

	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	for len(result) < len(viewMap) {
		if len(queue) == 0 {
			// Cycle detected: See detailed explanation in topologicallySortTables.
			// Views with circular dependencies are uncommon but possible via recursive CTEs
			// or mutual references in view definitions. We apply the same cycle-breaking
			// strategy: pick next in insertion order and set inDegree to 0.
			next := nextInOrder(insertionOrder, processed)
			if next == "" {
				break
			}
			queue = append(queue, next)
			inDegree[next] = 0
		}

		current := queue[0]
		queue = queue[1:]
		if processed[current] {
			continue
		}
		processed[current] = true
		result = append(result, current)

		neighbors := append([]string(nil), adjList[current]...)
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			// Add neighbor to queue if all its dependencies are satisfied.
			// The '!processed[neighbor]' check is critical: it prevents re-adding views
			// that have already been processed, even if their inDegree becomes <= 0 again
			// due to cycle breaking (where we artificially set inDegree to 0).
			if inDegree[neighbor] <= 0 && !processed[neighbor] {
				queue = append(queue, neighbor)
				sort.Strings(queue)
			}
		}
	}

	// Convert result back to view slice
	sortedViews := make([]*ir.View, 0, len(result))
	for _, key := range result {
		sortedViews = append(sortedViews, viewMap[key])
	}

	return sortedViews
}

// reverseSlice returns a new slice with elements in reverse order
func reverseSlice[T any](slice []T) []T {
	reversed := make([]T, len(slice))
	for i, v := range slice {
		reversed[len(slice)-1-i] = v
	}
	return reversed
}

func nextInOrder(order []string, processed map[string]bool) string {
	for _, key := range order {
		if !processed[key] {
			return key
		}
	}
	return ""
}

// topologicallySortTypes sorts types across all schemas in dependency order
// Types that are referenced by composite types will come before the types that reference them
func topologicallySortTypes(types []*ir.Type) []*ir.Type {
	if len(types) <= 1 {
		return types
	}

	// Build maps for efficient lookup
	typeMap := make(map[string]*ir.Type)
	var insertionOrder []string
	for _, t := range types {
		key := typeGraphKey(t.Schema, t.Name)
		typeMap[key] = t
		insertionOrder = append(insertionOrder, key)
	}

	// Build dependency graph
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for key := range typeMap {
		inDegree[key] = 0
		adjList[key] = []string{}
	}

	// Build edges: if typeA references typeB (composite type column uses typeB), add edge typeB -> typeA
	for keyA, typeA := range typeMap {
		if typeA.Kind == ir.TypeKindComposite {
			for _, col := range typeA.Columns {
				// Extract type name from DataType (may include schema prefix or array notation)
				referencedType := extractTypeName(col.DataType, typeA.Schema)
				if referencedType != "" {
					// Check if the referenced type exists in our set
					if _, exists := typeMap[referencedType]; exists && keyA != referencedType {
						adjList[referencedType] = append(adjList[referencedType], keyA)
						inDegree[keyA]++
					}
				}
			}
		} else if typeA.Kind == ir.TypeKindDomain {
			// Domain types may reference other types as their base type
			referencedType := extractTypeName(typeA.BaseType, typeA.Schema)
			if referencedType != "" {
				if _, exists := typeMap[referencedType]; exists && keyA != referencedType {
					adjList[referencedType] = append(adjList[referencedType], keyA)
					inDegree[keyA]++
				}
			}
		}
	}

	// Kahn's algorithm with deterministic cycle breaking
	var queue []string
	var result []string
	processed := make(map[string]bool, len(typeMap))

	// Seed queue with nodes that have no incoming edges
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	for len(result) < len(typeMap) {
		if len(queue) == 0 {
			// Cycle detected: pick the next unprocessed type using original insertion order
			//
			// CYCLE BREAKING STRATEGY FOR TYPES:
			// Setting inDegree[next] = 0 effectively declares "this type has no remaining dependencies"
			// for the purpose of breaking the cycle. This is safe because:
			//
			// 1. The 'processed' map prevents any type from being added to the result twice, even if
			//    its inDegree becomes zero or negative multiple times (see line 344 check).
			//
			// 2. For circular type dependencies in PostgreSQL, the dependency cycle can only occur
			//    through composite types referencing each other. Unlike table foreign keys, type
			//    dependencies cannot be added after creation - the entire type definition must be
			//    complete at CREATE TYPE time.
			//
			// 3. PostgreSQL itself prohibits creating types with true circular dependencies
			//    (composite type A containing type B, which contains type A) because it would
			//    result in infinite size. The only cycles that can occur in practice involve
			//    array types or indirection (e.g., A contains B[], B contains A[]), which
			//    PostgreSQL allows because arrays don't expand the size infinitely.
			//
			// 4. Using insertion order (alphabetical by schema.name) ensures deterministic output
			//    when multiple valid orderings exist.
			//
			// For types with unavoidable circular references (via arrays), the order doesn't
			// affect correctness since PostgreSQL's type system handles these internally.
			next := nextInOrder(insertionOrder, processed)
			if next == "" {
				break
			}
			queue = append(queue, next)
			inDegree[next] = 0
		}

		current := queue[0]
		queue = queue[1:]
		if processed[current] {
			continue
		}
		processed[current] = true
		result = append(result, current)

		neighbors := append([]string(nil), adjList[current]...)
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			// Add neighbor to queue if all its dependencies are satisfied.
			// The '!processed[neighbor]' check is critical: it prevents re-adding types
			// that have already been processed, even if their inDegree becomes <= 0 again
			// due to cycle breaking (where we artificially set inDegree to 0).
			if inDegree[neighbor] <= 0 && !processed[neighbor] {
				queue = append(queue, neighbor)
				sort.Strings(queue)
			}
		}
	}

	// Convert result back to type slice
	sortedTypes := make([]*ir.Type, 0, len(result))
	for _, key := range result {
		sortedTypes = append(sortedTypes, typeMap[key])
	}

	return sortedTypes
}

// typeGraphKey builds the lookup key used to identify a type in the dependency
// graph. It joins schema and name with a NUL byte, which cannot appear in a
// PostgreSQL identifier, so distinct (schema, name) pairs never collide even when
// an identifier legally contains a '.' (e.g. schema "a.b" type "c" vs schema "a"
// type "b.c").
func typeGraphKey(schema, name string) string {
	return schema + "\x00" + name
}

// extractTypeName maps a data type string to its dependency-graph key
// (see typeGraphKey). It handles array notation (e.g., "status_type[]") and
// schema prefixes.
func extractTypeName(dataType, defaultSchema string) string {
	if dataType == "" {
		return ""
	}

	// Remove array notation
	typeName := dataType
	for len(typeName) > 2 && typeName[len(typeName)-2:] == "[]" {
		typeName = typeName[:len(typeName)-2]
	}

	// Strip a trailing typmod suffix (e.g. "(10,2)") that the inspector appends
	// when qualifying user-defined scalar types, so it matches the bare typeMap
	// key. Only a purely numeric/comma/space typmod is stripped, so a quoted
	// identifier that legally contains parentheses (which ends in '"', not ')')
	// is left intact.
	if i := strings.LastIndexByte(typeName, '('); i > 0 && strings.HasSuffix(typeName, ")") {
		inner := typeName[i+1 : len(typeName)-1]
		if inner != "" && strings.IndexFunc(inner, func(r rune) bool {
			return !(r >= '0' && r <= '9' || r == ',' || r == ' ')
		}) == -1 {
			typeName = typeName[:i]
		}
	}

	// Check if it's a schema-qualified name. The inspector emits user-defined
	// type references via quote_ident (e.g. public."user"), so normalize each
	// component by stripping quote_ident quoting before building the key.
	if idx := findLastUnquotedDot(typeName); idx != -1 {
		return typeGraphKey(unquoteIdent(typeName[:idx]), unquoteIdent(typeName[idx+1:]))
	}

	// Not qualified - use default schema
	return typeGraphKey(unquoteIdent(defaultSchema), unquoteIdent(typeName))
}

// findLastUnquotedDot finds the last '.' that is not inside a double-quoted
// identifier, returning -1 if none. This avoids splitting on a dot that is part
// of a quoted identifier (e.g. the name in public."odd.name").
func findLastUnquotedDot(s string) int {
	inQuote := false
	last := -1
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case s[i] == '.' && !inQuote:
			last = i
		}
	}
	return last
}

// unquoteIdent removes quote_ident double-quoting from a single identifier
// (e.g. "user" -> user, "Weird""Name" -> Weird"Name). Bare identifiers are
// returned unchanged so this is safe on already-unquoted names.
func unquoteIdent(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return strings.ReplaceAll(s[1:len(s)-1], `""`, `"`)
	}
	return s
}

// topologicallySortFunctions sorts functions across all schemas in dependency order
// Functions that are referenced by other functions will come before the functions that reference them
func topologicallySortFunctions(functions []*ir.Function) []*ir.Function {
	if len(functions) <= 1 {
		return functions
	}

	// Build maps for efficient lookup
	funcMap := make(map[string]*ir.Function)
	var insertionOrder []string
	for _, fn := range functions {
		key := fn.Schema + "." + fn.Name + "(" + fn.GetArguments() + ")"
		funcMap[key] = fn
		insertionOrder = append(insertionOrder, key)
	}

	// Build dependency graph
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for key := range funcMap {
		inDegree[key] = 0
		adjList[key] = []string{}
	}

	// Build edges: if funcA depends on funcB, add edge funcB -> funcA
	for keyA, funcA := range funcMap {
		for _, depKey := range funcA.Dependencies {
			// depKey is already schema-qualified: schema.name(args)
			if _, exists := funcMap[depKey]; exists && keyA != depKey {
				adjList[depKey] = append(adjList[depKey], keyA)
				inDegree[keyA]++
			}
		}
	}

	// Kahn's algorithm with deterministic cycle breaking
	var queue []string
	var result []string
	processed := make(map[string]bool, len(funcMap))

	// Seed queue with nodes that have no incoming edges
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	for len(result) < len(funcMap) {
		if len(queue) == 0 {
			// Cycle detected: pick the next unprocessed function using original insertion order
			//
			// CYCLE BREAKING STRATEGY FOR FUNCTIONS:
			// Setting inDegree[next] = 0 effectively declares "this function has no remaining dependencies"
			// for the purpose of breaking the cycle. This is safe because:
			//
			// 1. The 'processed' map prevents any function from being added to the result twice, even if
			//    its inDegree becomes zero or negative multiple times (see processed[current] check below).
			//
			// 2. PostgreSQL allows mutually recursive functions through CREATE OR REPLACE FUNCTION.
			//    When functions A and B call each other, the creation order doesn't matter because
			//    PostgreSQL validates function bodies at call time, not at creation time (for most languages).
			//
			// 3. Using insertion order (alphabetical by schema.name(args)) ensures deterministic output
			//    when multiple valid orderings exist.
			//
			// This approach aligns with how PostgreSQL handles function dependencies - it doesn't
			// require strict ordering for mutually dependent functions.
			next := nextInOrder(insertionOrder, processed)
			if next == "" {
				break
			}
			queue = append(queue, next)
			inDegree[next] = 0
		}

		current := queue[0]
		queue = queue[1:]
		if processed[current] {
			continue
		}
		processed[current] = true
		result = append(result, current)

		neighbors := append([]string(nil), adjList[current]...)
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			if inDegree[neighbor] <= 0 && !processed[neighbor] {
				queue = append(queue, neighbor)
				sort.Strings(queue)
			}
		}
	}

	// Convert result back to function slice
	sortedFunctions := make([]*ir.Function, 0, len(result))
	for _, key := range result {
		sortedFunctions = append(sortedFunctions, funcMap[key])
	}

	return sortedFunctions
}

// topologicallySortModifiedTables sorts modified tables based on constraint dependencies
// Tables with added UNIQUE/PK constraints that are referenced by other tables' added FKs
// will come before those tables
func topologicallySortModifiedTables(tableDiffs []*tableDiff) []*tableDiff {
	if len(tableDiffs) <= 1 {
		return tableDiffs
	}

	// Build maps for efficient lookup
	tableDiffMap := make(map[string]*tableDiff)
	var insertionOrder []string
	for _, td := range tableDiffs {
		key := td.Table.Schema + "." + td.Table.Name
		tableDiffMap[key] = td
		insertionOrder = append(insertionOrder, key)
	}

	// Build dependency graph based on added constraints
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for key := range tableDiffMap {
		inDegree[key] = 0
		adjList[key] = []string{}
	}

	// Build edges: if tableA adds a FK to tableB's newly-added UNIQUE/PK, add edge tableB -> tableA
	for keyA, tdA := range tableDiffMap {
		// Look at FK constraints being added to tableA
		for _, fkConstraint := range tdA.AddedConstraints {
			if fkConstraint.Type != ir.ConstraintTypeForeignKey {
				continue
			}

			// Build referenced table key
			referencedSchema := fkConstraint.ReferencedSchema
			if referencedSchema == "" {
				referencedSchema = tdA.Table.Schema
			}
			keyB := referencedSchema + "." + fkConstraint.ReferencedTable

			// Check if referenced table exists in our modified tables set
			tdB, exists := tableDiffMap[keyB]
			if !exists || keyA == keyB {
				continue
			}

			// Check if tableB is adding a UNIQUE or PK constraint that matches the FK reference
			for _, constraint := range tdB.AddedConstraints {
				if constraint.Type != ir.ConstraintTypeUnique && constraint.Type != ir.ConstraintTypePrimaryKey {
					continue
				}

				// Check if this constraint matches the FK's referenced columns
				if constraintMatchesFKReference(constraint, fkConstraint) {
					// Add edge: tableB (with new UNIQUE/PK) -> tableA (with new FK)
					adjList[keyB] = append(adjList[keyB], keyA)
					inDegree[keyA]++
					break
				}
			}
		}
	}

	// Kahn's algorithm with deterministic cycle breaking
	var queue []string
	var result []string
	processed := make(map[string]bool, len(tableDiffMap))

	// Seed queue with nodes that have no incoming edges
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)

	for len(result) < len(tableDiffMap) {
		if len(queue) == 0 {
			// Cycle detected: pick the next unprocessed table using original insertion order
			next := nextInOrder(insertionOrder, processed)
			if next == "" {
				break
			}
			queue = append(queue, next)
			inDegree[next] = 0
		}

		current := queue[0]
		queue = queue[1:]
		if processed[current] {
			continue
		}
		processed[current] = true
		result = append(result, current)

		neighbors := append([]string(nil), adjList[current]...)
		sort.Strings(neighbors)

		for _, neighbor := range neighbors {
			inDegree[neighbor]--
			if inDegree[neighbor] <= 0 && !processed[neighbor] {
				queue = append(queue, neighbor)
				sort.Strings(queue)
			}
		}
	}

	// Convert result back to tableDiff slice
	sortedTableDiffs := make([]*tableDiff, 0, len(result))
	for _, key := range result {
		sortedTableDiffs = append(sortedTableDiffs, tableDiffMap[key])
	}

	return sortedTableDiffs
}

// constraintMatchesFKReference checks if a UNIQUE/PK constraint matches the columns
// referenced by a foreign key constraint.
// In PostgreSQL, composite foreign keys must reference columns in the same order as they
// appear in the referenced unique/primary key constraint.
// For example, FK (col1, col2) can only reference UNIQUE (col1, col2), not UNIQUE (col2, col1).
func constraintMatchesFKReference(uniqueConstraint, fkConstraint *ir.Constraint) bool {
	// Must have same number of columns
	if len(uniqueConstraint.Columns) != len(fkConstraint.ReferencedColumns) {
		return false
	}

	// Sort both constraint columns by position to ensure order-preserving comparison
	uniqueCols := sortConstraintColumnsByPosition(uniqueConstraint.Columns)
	refCols := sortConstraintColumnsByPosition(fkConstraint.ReferencedColumns)

	// Check if columns match in the same order (position by position)
	for i := 0; i < len(uniqueCols); i++ {
		if uniqueCols[i].Name != refCols[i].Name {
			return false
		}
	}

	return true
}

// buildFunctionBodyDependencies scans function bodies for function calls and populates
// the Dependencies field. This supplements dependencies from pg_depend, which doesn't
// track references inside SQL function bodies.
func buildFunctionBodyDependencies(functions []*ir.Function) {
	if len(functions) <= 1 {
		return
	}

	// Build lookup maps by function name (both qualified and unqualified)
	// Map to the full key format used by Dependencies: schema.name(args)
	type funcInfo struct {
		fn  *ir.Function
		key string
	}
	functionLookup := make(map[string]funcInfo)

	for _, fn := range functions {
		key := fn.Schema + "." + fn.Name + "(" + fn.GetArguments() + ")"
		name := strings.ToLower(fn.Name)

		// Store under unqualified name
		functionLookup[name] = funcInfo{fn: fn, key: key}

		// Store under qualified name
		if fn.Schema != "" {
			qualified := strings.ToLower(fn.Schema) + "." + name
			functionLookup[qualified] = funcInfo{fn: fn, key: key}
		}
	}

	// For each function, scan its body for function calls
	for _, fn := range functions {
		if fn.Definition == "" {
			continue
		}

		fnKey := fn.Schema + "." + fn.Name + "(" + fn.GetArguments() + ")"

		matches := functionCallRegex.FindAllStringSubmatch(fn.Definition, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			identifier := strings.ToLower(match[1])
			if identifier == "" {
				continue
			}

			// Try to find the referenced function
			var info funcInfo
			var found bool

			if info, found = functionLookup[identifier]; !found {
				// Try with schema prefix if identifier is unqualified
				if !strings.Contains(identifier, ".") && fn.Schema != "" {
					qualified := strings.ToLower(fn.Schema) + "." + identifier
					info, found = functionLookup[qualified]
				}
			}

			// If found and not self-reference, add dependency
			if found && info.key != fnKey {
				// Check if dependency already exists
				alreadyExists := false
				for _, existing := range fn.Dependencies {
					if existing == info.key {
						alreadyExists = true
						break
					}
				}
				if !alreadyExists {
					fn.Dependencies = append(fn.Dependencies, info.key)
				}
			}
		}
	}
}
