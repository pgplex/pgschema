package ir

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// normalizeIR normalizes the IR representation from the inspector.
//
// Since both desired state (from embedded postgres) and current state (from target database)
// now come from the same PostgreSQL version via database inspection, most normalizations
// are no longer needed. The remaining normalizations handle:
//
// - Type name mappings (internal PostgreSQL types → standard SQL types, e.g., int4 → integer)
// - PostgreSQL internal representations (e.g., "~~ " → "LIKE", "= ANY (ARRAY[...])" → "IN (...)")
// - Minor formatting differences in default values, policies, triggers, etc.
func normalizeIR(ir *IR) {
	if ir == nil {
		return
	}

	for _, schema := range ir.Schemas {
		normalizeSchema(schema)
	}
}

// normalizeSchema normalizes all objects within a schema
func normalizeSchema(schema *Schema) {
	if schema == nil {
		return
	}

	// Normalize tables
	for _, table := range schema.Tables {
		normalizeTable(table)
	}

	// Normalize views
	for _, view := range schema.Views {
		normalizeView(view)
	}

	// Normalize functions
	for _, function := range schema.Functions {
		normalizeFunction(function)
	}

	// Normalize procedures
	for _, procedure := range schema.Procedures {
		normalizeProcedure(procedure)
	}

	// Normalize types (including domains)
	for _, typeObj := range schema.Types {
		normalizeType(typeObj)
	}
}

// normalizeTable normalizes table-related objects
func normalizeTable(table *Table) {
	if table == nil {
		return
	}

	// Normalize columns (pass table schema for context)
	for _, column := range table.Columns {
		normalizeColumn(column, table.Schema)
	}

	// Normalize policies (pass table schema for context - Issue #220)
	for _, policy := range table.Policies {
		normalizePolicy(policy, table.Schema)
	}

	// Normalize triggers
	for _, trigger := range table.Triggers {
		normalizeTrigger(trigger)
	}

	// Normalize indexes
	for _, index := range table.Indexes {
		normalizeIndex(index)
	}

	// Normalize constraints
	for _, constraint := range table.Constraints {
		normalizeConstraint(constraint)
	}
}

// normalizeColumn normalizes column default values
// tableSchema is used to strip same-schema qualifiers from function calls
func normalizeColumn(column *Column, tableSchema string) {
	if column == nil || column.DefaultValue == nil {
		return
	}

	normalized := normalizeDefaultValue(*column.DefaultValue, tableSchema)
	column.DefaultValue = &normalized
}

// normalizeDefaultValue normalizes default values for semantic comparison
// tableSchema is used to strip same-schema qualifiers from function calls
func normalizeDefaultValue(value string, tableSchema string) string {
	// Remove unnecessary whitespace
	value = strings.TrimSpace(value)

	// Handle nextval sequence references - remove schema qualification
	if strings.Contains(value, "nextval(") {
		// Pattern: nextval('schema_name.seq_name'::regclass) -> nextval('seq_name'::regclass)
		re := regexp.MustCompile(`nextval\('([^.]+)\.([^']+)'::regclass\)`)
		if re.MatchString(value) {
			// Replace with unqualified sequence name
			value = re.ReplaceAllString(value, "nextval('$2'::regclass)")
		}
		// Early return for nextval - don't apply type casting normalization
		return value
	}

	// Normalize function calls - remove schema qualifiers for functions in the same schema
	// This matches PostgreSQL's pg_get_expr() behavior which strips same-schema qualifiers
	// Example: public.get_status() -> get_status() (when tableSchema is "public")
	//          other_schema.get_status() -> other_schema.get_status() (preserved)
	if tableSchema != "" && strings.Contains(value, tableSchema+".") {
		// Pattern: schema.function_name(
		// Replace "tableSchema." with "" when followed by identifier and (
		prefix := tableSchema + "."
		pattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([a-zA-Z_][a-zA-Z0-9_]*)\(`)
		value = pattern.ReplaceAllString(value, `${1}(`)
	}

	// Handle type casting - remove explicit type casts that are semantically equivalent
	if strings.Contains(value, "::") {
		// Strip temporary embedded postgres schema prefixes (pgschema_tmp_*)
		// These are used internally during plan generation and should be normalized away
		// Pattern: ::pgschema_tmp_YYYYMMDD_HHMMSS_XXXXXXXX.typename -> ::typename
		if strings.Contains(value, "::pgschema_tmp_") {
			re := regexp.MustCompile(`::pgschema_tmp_[^.]+\.`)
			value = re.ReplaceAllString(value, "::")
		}
		// Also strip same-schema type qualifiers for consistent comparison during plan/diff
		// This ensures that '::public.typename' from current state matches '::typename' from
		// desired state (after pgschema_tmp_* is stripped). Both are semantically equivalent
		// within the same schema context. (Issue #218)
		if tableSchema != "" && strings.Contains(value, "::"+tableSchema+".") {
			re := regexp.MustCompile(`::\Q` + tableSchema + `\E\.`)
			value = re.ReplaceAllString(value, "::")
		}

		// Handle NULL::type -> NULL
		// Example: NULL::text -> NULL
		re := regexp.MustCompile(`\bNULL::(?:[a-zA-Z_][\w\s.]*)(?:\[\])?`)
		value = re.ReplaceAllString(value, "NULL")

		// Handle numeric literals with type casts
		// Example: '-1'::integer -> -1
		// Example: '100'::bigint -> 100
		// Note: PostgreSQL sometimes casts numeric literals to different types, e.g., -1::integer stored as numeric
		re = regexp.MustCompile(`'(-?\d+(?:\.\d+)?)'::(?:integer|bigint|smallint|numeric|decimal|real|double precision|int2|int4|int8|float4|float8)`)
		value = re.ReplaceAllString(value, "$1")

		// Handle string literals with ONLY truly redundant type casts
		// Only remove casts where the literal is inherently the target type:
		// Example: 'text'::text -> 'text' (string literal IS text)
		// Example: 'O''Brien'::character varying -> 'O''Brien'
		// Example: '{}'::text[] -> '{}' (empty array literal with text array cast)
		// Example: '{}'::jsonb -> '{}' (JSON object literal - column type provides context)
		//
		// IMPORTANT: Do NOT remove semantically significant casts like:
		// - '1 year'::interval (interval literals REQUIRE the cast)
		// - 'value'::my_enum (custom type casts)
		// - '2024-01-01'::date (date literals need the cast in expressions)
		//
		// Pattern matches redundant text/varchar/char/json casts (including arrays)
		// For column defaults, these casts are redundant because the column type provides context
		// Note: jsonb must come before json to avoid partial match
		// Note: (?:\[\])* handles multi-dimensional arrays like text[][]
		re = regexp.MustCompile(`('(?:[^']|'')*')::(text|character varying|character|bpchar|varchar|jsonb|json)(?:\[\])*`)
		value = re.ReplaceAllString(value, "$1")

		// Handle parenthesized expressions with type casts - remove outer parentheses
		// Example: (100)::bigint -> 100::bigint
		// Pattern captures the number and the type cast separately
		re = regexp.MustCompile(`\((\d+)\)(::(?:bigint|integer|smallint|numeric|decimal))`)
		value = re.ReplaceAllString(value, "$1$2")
	}

	return value
}

// normalizePolicy normalizes RLS policy representation
// tableSchema is used to strip same-schema qualifiers from function calls (Issue #220)
func normalizePolicy(policy *RLSPolicy, tableSchema string) {
	if policy == nil {
		return
	}

	// Normalize roles - ensure consistent ordering and case
	policy.Roles = normalizePolicyRoles(policy.Roles)

	// Normalize expressions by removing extra whitespace
	// For policy expressions, we want to preserve parentheses as they are part of the expected format
	policy.Using = normalizePolicyExpression(policy.Using, tableSchema)
	policy.WithCheck = normalizePolicyExpression(policy.WithCheck, tableSchema)
}

// normalizePolicyRoles normalizes policy roles for consistent comparison
func normalizePolicyRoles(roles []string) []string {
	if len(roles) == 0 {
		return roles
	}

	// Normalize role names with special handling for PUBLIC
	normalized := make([]string, len(roles))
	for i, role := range roles {
		// Keep PUBLIC in uppercase, normalize others to lowercase
		if strings.ToUpper(role) == "PUBLIC" {
			normalized[i] = "PUBLIC"
		} else {
			normalized[i] = strings.ToLower(role)
		}
	}

	// Sort to ensure consistent ordering
	sort.Strings(normalized)
	return normalized
}

// normalizePolicyExpression normalizes policy expressions (USING/WITH CHECK clauses)
// It preserves parentheses as they are part of the expected format for policies
// tableSchema is used to strip same-schema qualifiers from function calls and table references (Issue #220, #224)
func normalizePolicyExpression(expr string, tableSchema string) string {
	if expr == "" {
		return expr
	}

	// Remove extra whitespace and normalize
	expr = strings.TrimSpace(expr)
	expr = regexp.MustCompile(`\s+`).ReplaceAllString(expr, " ")

	// Strip same-schema qualifiers from function calls (Issue #220)
	// This matches PostgreSQL's behavior where same-schema qualifiers are stripped
	// Example: tenant1.auth_uid() -> auth_uid() (when tableSchema is "tenant1")
	//          util.get_status() -> util.get_status() (preserved, different schema)
	if tableSchema != "" && strings.Contains(expr, tableSchema+".") {
		prefix := tableSchema + "."
		pattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([a-zA-Z_][a-zA-Z0-9_]*)\(`)
		expr = pattern.ReplaceAllString(expr, `${1}(`)

		// Strip same-schema qualifiers from table references (Issue #224)
		// Matches schema.identifier followed by whitespace, comma, closing paren, or end of string
		// Example: public.users -> users (when tableSchema is "public")
		tablePattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([a-zA-Z_][a-zA-Z0-9_]*)(\s|,|\)|$)`)
		expr = tablePattern.ReplaceAllString(expr, `${1}${2}`)
	}

	// Handle all parentheses normalization (adding required ones, removing unnecessary ones)
	expr = normalizeExpressionParentheses(expr)

	// Normalize PostgreSQL internal type names to standard SQL types
	expr = normalizePostgreSQLType(expr)

	return expr
}

// normalizeView normalizes view definition.
//
// While both desired state (from embedded postgres) and current state (from target database)
// come from pg_get_viewdef(), they may differ in schema qualification of functions and tables.
// This happens when extension functions (e.g., ltree's nlevel()) or search_path differences
// cause one side to produce "public.func()" and the other "func()".
// Stripping same-schema qualifiers ensures the definitions compare as equal. (Issue #314)
func normalizeView(view *View) {
	if view == nil {
		return
	}

	// Strip same-schema qualifiers from view definition for consistent comparison.
	// This uses the same logic as function/procedure body normalization.
	view.Definition = stripSchemaPrefixFromBody(view.Definition, view.Schema)

	// Normalize triggers on the view (e.g., INSTEAD OF triggers)
	for _, trigger := range view.Triggers {
		normalizeTrigger(trigger)
	}
}

// normalizeFunction normalizes function signature and definition
func normalizeFunction(function *Function) {
	if function == nil {
		return
	}

	// lowercase LANGUAGE plpgsql is more common in modern usage
	function.Language = strings.ToLower(function.Language)
	// Normalize return type to handle PostgreSQL-specific formats
	function.ReturnType = normalizeFunctionReturnType(function.ReturnType)
	// Strip current schema qualifier from return type for consistent comparison.
	// pg_get_function_result may or may not qualify types in the current schema
	// depending on search_path (e.g., "SETOF public.actor" vs "SETOF actor").
	function.ReturnType = stripSchemaFromReturnType(function.ReturnType, function.Schema)
	// Normalize parameter types, modes, and default values
	for _, param := range function.Parameters {
		if param != nil {
			param.DataType = normalizePostgreSQLType(param.DataType)
			// Normalize mode: empty string → "IN" for functions (PostgreSQL default)
			// Functions: IN is default, only OUT/INOUT/VARIADIC need explicit mode
			// But for consistent comparison, normalize empty to "IN"
			if param.Mode == "" {
				param.Mode = "IN"
			}
			// Normalize default values (pass function schema for context)
			if param.DefaultValue != nil {
				normalized := normalizeDefaultValue(*param.DefaultValue, function.Schema)
				param.DefaultValue = &normalized
			}
		}
	}
	// Normalize function body to handle whitespace differences
	function.Definition = normalizeFunctionDefinition(function.Definition)
	// Strip current schema qualifier from function body for consistent unqualified output
	function.Definition = stripSchemaPrefixFromBody(function.Definition, function.Schema)
}

// normalizeFunctionDefinition normalizes function body whitespace
// PostgreSQL stores function bodies with specific whitespace that may differ from source
func normalizeFunctionDefinition(def string) string {
	if def == "" {
		return def
	}

	// Only trim trailing whitespace from each line, preserving the line structure
	// This ensures leading/trailing blank lines are preserved (matching PostgreSQL storage)
	lines := strings.Split(def, "\n")
	var normalized []string
	for _, line := range lines {
		// Trim all trailing whitespace (spaces, tabs, CR) but preserve leading whitespace for indentation
		normalized = append(normalized, strings.TrimRightFunc(line, unicode.IsSpace))
	}

	return strings.Join(normalized, "\n")
}

// stripSchemaPrefixFromBody removes the current schema qualifier from identifiers
// in a function or procedure body. For example, "public.users" becomes "users".
// It skips single-quoted string literals to avoid modifying string constants.
func stripSchemaPrefixFromBody(body, schema string) string {
	if body == "" || schema == "" {
		return body
	}

	prefix := schema + "."
	prefixLen := len(prefix)

	// Fast path: if the prefix doesn't appear at all, return as-is
	if !strings.Contains(body, prefix) {
		return body
	}

	var result strings.Builder
	result.Grow(len(body))
	inString := false

	for i := 0; i < len(body); i++ {
		ch := body[i]

		// Track single-quoted string literals, handling '' escapes
		if ch == '\'' {
			if inString {
				if i+1 < len(body) && body[i+1] == '\'' {
					// Escaped quote inside string: write both and skip
					result.WriteString("''")
					i++
					continue
				}
				inString = false
			} else {
				inString = true
			}
			result.WriteByte(ch)
			continue
		}

		// Only attempt replacement outside string literals
		if !inString && i+prefixLen <= len(body) && body[i:i+prefixLen] == prefix {
			// Ensure this is a schema qualifier, not part of a longer identifier
			// (e.g., "not_public.users" should not match)
			if i == 0 || !isIdentChar(body[i-1]) {
				// After stripping the schema prefix, check if the remaining identifier
				// is a reserved keyword that needs quoting.
				// e.g., public.user → "user", public.order → "order"
				afterPrefix := i + prefixLen
				identEnd := afterPrefix
				for identEnd < len(body) && isIdentChar(body[identEnd]) {
					identEnd++
				}
				ident := body[afterPrefix:identEnd]
				if needsQuoting(ident) {
					result.WriteString(QuoteIdentifier(ident))
					i = identEnd - 1
					continue
				}
				// Skip the schema prefix, keep everything after it
				i += prefixLen - 1
				continue
			}
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// isIdentChar returns true if the byte is a valid SQL identifier character.
func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// normalizeProcedure normalizes procedure representation
func normalizeProcedure(procedure *Procedure) {
	if procedure == nil {
		return
	}

	// Normalize language to lowercase (PLPGSQL → plpgsql)
	procedure.Language = strings.ToLower(procedure.Language)

	// Normalize parameter types, modes, and default values
	for _, param := range procedure.Parameters {
		if param != nil {
			param.DataType = normalizePostgreSQLType(param.DataType)
			// Normalize mode: empty string → "IN" for procedures (PostgreSQL default)
			if param.Mode == "" {
				param.Mode = "IN"
			}
			// Normalize default values (pass procedure schema for context)
			if param.DefaultValue != nil {
				normalized := normalizeDefaultValue(*param.DefaultValue, procedure.Schema)
				param.DefaultValue = &normalized
			}
		}
	}

	// Strip current schema qualifier from procedure body for consistent unqualified output
	procedure.Definition = stripSchemaPrefixFromBody(procedure.Definition, procedure.Schema)
}

// normalizeFunctionReturnType normalizes function return types, especially TABLE types
func normalizeFunctionReturnType(returnType string) string {
	if returnType == "" {
		return returnType
	}

	// Handle TABLE return types
	if strings.HasPrefix(returnType, "TABLE(") && strings.HasSuffix(returnType, ")") {
		// Extract the contents inside TABLE(...)
		inner := returnType[6 : len(returnType)-1] // Remove "TABLE(" and ")"

		// Split by comma to process each column definition
		parts := strings.Split(inner, ",")
		var normalizedParts []string

		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			// Normalize individual column definitions (name type)
			fields := strings.Fields(part)
			if len(fields) >= 2 {
				// Normalize the type part
				typePart := strings.Join(fields[1:], " ")
				normalizedType := normalizePostgreSQLType(typePart)
				normalizedParts = append(normalizedParts, fields[0]+" "+normalizedType)
			} else {
				// Just a type, normalize it
				normalizedParts = append(normalizedParts, normalizePostgreSQLType(part))
			}
		}

		return "TABLE(" + strings.Join(normalizedParts, ", ") + ")"
	}

	// For non-TABLE return types, apply regular type normalization
	return normalizePostgreSQLType(returnType)
}

// stripSchemaFromReturnType strips the current schema qualifier from a function return type.
// This handles SETOF and array types, e.g., "SETOF public.actor" → "SETOF actor"
// when the function is in the public schema.
func stripSchemaFromReturnType(returnType, schema string) string {
	if returnType == "" || schema == "" {
		return returnType
	}

	prefix := schema + "."

	// Handle SETOF prefix
	if len(returnType) > 6 && strings.EqualFold(returnType[:6], "SETOF ") {
		rest := strings.TrimSpace(returnType[6:])
		stripped := stripSchemaPrefix(rest, prefix)
		if stripped != rest {
			return returnType[:6] + stripped
		}
		return returnType
	}

	// Handle TABLE(...) return types - strip schema from individual column types
	if strings.HasPrefix(returnType, "TABLE(") {
		return returnType // TABLE types are already handled by normalizeFunctionReturnType
	}

	// Direct type name
	return stripSchemaPrefix(returnType, prefix)
}

// stripSchemaPrefix removes a schema prefix from a type name, preserving array notation.
func stripSchemaPrefix(typeName, prefix string) string {
	// Separate base type from array suffix (e.g., "public.mytype[]" → "public.mytype" + "[]")
	base := typeName
	arrayStart := strings.Index(base, "[]")
	arraySuffix := ""
	if arrayStart >= 0 {
		arraySuffix = base[arrayStart:]
		base = base[:arrayStart]
	}

	if strings.HasPrefix(base, prefix) {
		return base[len(prefix):] + arraySuffix
	}
	return typeName
}

// normalizeTrigger normalizes trigger representation
func normalizeTrigger(trigger *Trigger) {
	if trigger == nil {
		return
	}

	// Normalize trigger function call with the trigger's schema context
	trigger.Function = normalizeTriggerFunctionCall(trigger.Function, trigger.Schema)

	// Normalize trigger events to standard order: INSERT, UPDATE, DELETE, TRUNCATE
	trigger.Events = normalizeTriggerEvents(trigger.Events)

	// Normalize trigger condition (WHEN clause) for consistent comparison
	trigger.Condition = normalizeTriggerCondition(trigger.Condition)
}

// normalizeTriggerFunctionCall normalizes trigger function call syntax and removes same-schema qualifiers
func normalizeTriggerFunctionCall(functionCall string, triggerSchema string) string {
	if functionCall == "" {
		return functionCall
	}

	// Remove extra whitespace
	functionCall = strings.TrimSpace(functionCall)
	functionCall = regexp.MustCompile(`\s+`).ReplaceAllString(functionCall, " ")

	// Normalize function call formatting
	functionCall = regexp.MustCompile(`\(\s*`).ReplaceAllString(functionCall, "(")
	functionCall = regexp.MustCompile(`\s*\)`).ReplaceAllString(functionCall, ")")
	functionCall = regexp.MustCompile(`\s*,\s*`).ReplaceAllString(functionCall, ", ")

	// Strip schema qualifier if it matches the trigger's schema
	if triggerSchema != "" {
		schemaPrefix := triggerSchema + "."
		functionCall = strings.TrimPrefix(functionCall, schemaPrefix)
	}

	return functionCall
}

// normalizeTriggerEvents normalizes trigger events to standard order
func normalizeTriggerEvents(events []TriggerEvent) []TriggerEvent {
	if len(events) == 0 {
		return events
	}

	// Define standard order: INSERT, UPDATE, DELETE, TRUNCATE
	standardOrder := []TriggerEvent{
		TriggerEventInsert,
		TriggerEventUpdate,
		TriggerEventDelete,
		TriggerEventTruncate,
	}

	// Create a set of events for quick lookup
	eventSet := make(map[TriggerEvent]bool)
	for _, event := range events {
		eventSet[event] = true
	}

	// Build normalized events in standard order
	var normalized []TriggerEvent
	for _, event := range standardOrder {
		if eventSet[event] {
			normalized = append(normalized, event)
		}
	}

	return normalized
}

// normalizeTriggerCondition normalizes trigger WHEN conditions for consistent comparison
func normalizeTriggerCondition(condition string) string {
	if condition == "" {
		return condition
	}

	// Normalize whitespace
	condition = strings.TrimSpace(condition)
	condition = regexp.MustCompile(`\s+`).ReplaceAllString(condition, " ")

	// Normalize NEW and OLD identifiers to uppercase
	condition = regexp.MustCompile(`\bnew\b`).ReplaceAllStringFunc(condition, func(match string) string {
		return strings.ToUpper(match)
	})
	condition = regexp.MustCompile(`\bold\b`).ReplaceAllStringFunc(condition, func(match string) string {
		return strings.ToUpper(match)
	})

	// PostgreSQL stores "IS NOT DISTINCT FROM" as "NOT (... IS DISTINCT FROM ...)"
	// Convert the internal form to the SQL standard form for consistency
	// Pattern: NOT (expr IS DISTINCT FROM expr) -> expr IS NOT DISTINCT FROM expr
	re := regexp.MustCompile(`NOT \((.+?)\s+IS\s+DISTINCT\s+FROM\s+(.+?)\)`)
	condition = re.ReplaceAllString(condition, "$1 IS NOT DISTINCT FROM $2")

	return condition
}

// normalizeIndex normalizes index WHERE clauses and other properties
func normalizeIndex(index *Index) {
	if index == nil {
		return
	}

	// Normalize WHERE clause for partial indexes
	if index.IsPartial && index.Where != "" {
		index.Where = normalizeIndexWhereClause(index.Where)
	}
}

// normalizeIndexWhereClause normalizes WHERE clauses in partial indexes
// It handles proper parentheses for different expression types
func normalizeIndexWhereClause(where string) string {
	if where == "" {
		return where
	}

	// Remove any existing outer parentheses to normalize the input
	if strings.HasPrefix(where, "(") && strings.HasSuffix(where, ")") {
		// Check if the parentheses wrap the entire expression
		inner := where[1 : len(where)-1]
		if isBalancedParentheses(inner) {
			where = inner
		}
	}

	// Convert PostgreSQL's "= ANY (ARRAY[...])" format to "IN (...)" format
	where = convertAnyArrayToIn(where)

	// Determine if this expression needs outer parentheses based on its structure
	needsParentheses := shouldAddParenthesesForWhereClause(where)

	if needsParentheses {
		return fmt.Sprintf("(%s)", where)
	}

	return where
}

// shouldAddParenthesesForWhereClause determines if a WHERE clause needs outer parentheses
// Based on PostgreSQL's formatting expectations for pg_get_expr
func shouldAddParenthesesForWhereClause(expr string) bool {
	if expr == "" {
		return false
	}

	// Don't add parentheses for well-formed expressions that are self-contained:

	// 1. IN expressions: "column IN (value1, value2, value3)"
	if strings.Contains(expr, " IN (") {
		return false
	}

	// 2. Function calls: "function_name(args)"
	if matches, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*\s*\(.*\)$`, expr); matches {
		return false
	}

	// 3. Simple comparisons with parenthesized right side: "column = (value)"
	if matches, _ := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*\s*[=<>!]+\s*\(.*\)$`, expr); matches {
		return false
	}

	// 4. Already fully parenthesized complex expressions
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return false
	}

	// For other expressions (simple comparisons, AND/OR combinations, etc.), add parentheses
	return true
}

// normalizeExpressionParentheses handles parentheses normalization for policy expressions
// It ensures required parentheses for PostgreSQL DDL while removing unnecessary ones
func normalizeExpressionParentheses(expr string) string {
	if expr == "" {
		return expr
	}

	// Step 1: Ensure WITH CHECK/USING expressions are properly parenthesized
	// PostgreSQL requires parentheses around all policy expressions in DDL
	if !strings.HasPrefix(expr, "(") || !strings.HasSuffix(expr, ")") {
		expr = fmt.Sprintf("(%s)", expr)
	}

	// Step 2: Remove unnecessary parentheses around function calls within the expression
	// Specifically targets patterns like (function_name(...)) -> function_name(...)
	// This pattern looks for:
	// \( - opening parenthesis
	// ([a-zA-Z_][a-zA-Z0-9_]*) - function name (captured)
	// \( - opening parenthesis for function call
	// ([^)]*) - function arguments (captured, non-greedy to avoid matching nested parens)
	// \) - closing parenthesis for function call
	// \) - closing parenthesis around the whole function
	functionParensRegex := regexp.MustCompile(`\(([a-zA-Z_][a-zA-Z0-9_]*\([^)]*\))\)`)

	// Replace (function(...)) with function(...)
	// Keep applying until no more matches to handle nested cases
	for {
		original := expr
		expr = functionParensRegex.ReplaceAllString(expr, "$1")
		if expr == original {
			break
		}
	}

	// Step 3: Normalize redundant type casts in function arguments
	// Pattern: 'text'::text -> 'text' (removing redundant text cast from literals)
	// IMPORTANT: Do NOT match when followed by [] (array cast is semantically significant)
	// e.g., '{nested,key}'::text[] must be preserved as-is
	// Since Go regex doesn't support lookahead, we use [^[\w] which excludes both '['
	// and word characters (letters/digits/_), correctly preventing matches like ::text[] or ::textual
	redundantTextCastRegex := regexp.MustCompile(`'([^']+)'::text([^[\w]|$)`)
	expr = redundantTextCastRegex.ReplaceAllString(expr, "'$1'$2")

	return expr
}

// isBalancedParentheses checks if parentheses are properly balanced in the expression
func isBalancedParentheses(expr string) bool {
	count := 0
	inQuotes := false
	var quoteChar rune

	for _, r := range expr {
		if !inQuotes {
			switch r {
			case '\'', '"':
				inQuotes = true
				quoteChar = r
			case '(':
				count++
			case ')':
				count--
				if count < 0 {
					return false
				}
			}
		} else {
			if r == quoteChar {
				inQuotes = false
			}
		}
	}

	return count == 0
}

// normalizeType normalizes type-related objects, including domain constraints
func normalizeType(typeObj *Type) {
	if typeObj == nil || typeObj.Kind != TypeKindDomain {
		return
	}

	// Normalize domain default value
	if typeObj.Default != "" {
		typeObj.Default = normalizeDomainDefault(typeObj.Default)
	}

	// Normalize domain constraints (pass schema for stripping same-schema qualifiers)
	for _, constraint := range typeObj.Constraints {
		normalizeDomainConstraint(constraint, typeObj.Schema)
	}
}

// normalizeDomainDefault normalizes domain default values
func normalizeDomainDefault(defaultValue string) string {
	if defaultValue == "" {
		return defaultValue
	}

	// Remove redundant type casts from string literals
	// e.g., 'example@acme.com'::text -> 'example@acme.com'
	defaultValue = regexp.MustCompile(`'([^']+)'::text\b`).ReplaceAllString(defaultValue, "'$1'")

	return defaultValue
}

// normalizeDomainConstraint normalizes domain constraint definitions
// domainSchema is used to strip same-schema qualifiers from function calls
func normalizeDomainConstraint(constraint *DomainConstraint, domainSchema string) {
	if constraint == nil || constraint.Definition == "" {
		return
	}

	def := constraint.Definition

	// Normalize VALUE keyword to uppercase in domain constraints
	// Use word boundaries to ensure we only match the identifier, not parts of other words
	def = regexp.MustCompile(`\bvalue\b`).ReplaceAllStringFunc(def, func(match string) string {
		return strings.ToUpper(match)
	})

	// Handle CHECK constraints
	if strings.HasPrefix(def, "CHECK ") {
		// Extract the expression inside CHECK (...)
		checkMatch := regexp.MustCompile(`^CHECK\s*\((.*)\)$`).FindStringSubmatch(def)
		if len(checkMatch) > 1 {
			expr := checkMatch[1]

			// Remove outer parentheses if they wrap the entire expression
			expr = strings.TrimSpace(expr)
			if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
				inner := expr[1 : len(expr)-1]
				if isBalancedParentheses(inner) {
					expr = inner
				}
			}

			// Strip same-schema qualifiers from function calls (similar to normalizePolicyExpression)
			// This matches PostgreSQL's behavior where pg_get_constraintdef includes schema qualifiers
			// but the source SQL may not include them
			// Example: public.validate_custom_id(VALUE) -> validate_custom_id(VALUE) (when domainSchema is "public")
			if domainSchema != "" && strings.Contains(expr, domainSchema+".") {
				prefix := domainSchema + "."
				pattern := regexp.MustCompile(regexp.QuoteMeta(prefix) + `([a-zA-Z_][a-zA-Z0-9_]*)\(`)
				expr = pattern.ReplaceAllString(expr, `${1}(`)
			}

			// Remove redundant type casts
			// e.g., '...'::text -> '...'
			expr = regexp.MustCompile(`'([^']+)'::text\b`).ReplaceAllString(expr, "'$1'")

			// Reconstruct the CHECK constraint
			def = fmt.Sprintf("CHECK (%s)", expr)
		}
	}

	constraint.Definition = def
}

// postgresTypeNormalization maps PostgreSQL internal type names to standard SQL types.
// This map is used by normalizePostgreSQLType to normalize type representations.
var postgresTypeNormalization = map[string]string{
	// Numeric types
	"int2":               "smallint",
	"int4":               "integer",
	"int8":               "bigint",
	"float4":             "real",
	"float8":             "double precision",
	"bool":               "boolean",
	"pg_catalog.int2":    "smallint",
	"pg_catalog.int4":    "integer",
	"pg_catalog.int8":    "bigint",
	"pg_catalog.float4":  "real",
	"pg_catalog.float8":  "double precision",
	"pg_catalog.bool":    "boolean",
	"pg_catalog.numeric": "numeric",

	// Character types
	"bpchar":             "character",
	"character varying":  "varchar", // Prefer short form
	"pg_catalog.text":    "text",
	"pg_catalog.varchar": "varchar", // Prefer short form
	"pg_catalog.bpchar":  "character",

	// Date/time types - convert verbose forms to canonical short forms
	"timestamp with time zone":    "timestamptz",
	"timestamp without time zone": "timestamp",
	"time with time zone":         "timetz",
	"timestamptz":                 "timestamptz",
	"timetz":                      "timetz",
	"pg_catalog.timestamptz":      "timestamptz",
	"pg_catalog.timestamp":        "timestamp",
	"pg_catalog.date":             "date",
	"pg_catalog.time":             "time",
	"pg_catalog.timetz":           "timetz",
	"pg_catalog.interval":         "interval",

	// Array types (internal PostgreSQL array notation with underscore prefix)
	"_text":        "text[]",
	"_int2":        "smallint[]",
	"_int4":        "integer[]",
	"_int8":        "bigint[]",
	"_float4":      "real[]",
	"_float8":      "double precision[]",
	"_bool":        "boolean[]",
	"_varchar":     "varchar[]", // Prefer short form
	"_char":        "character[]",
	"_bpchar":      "character[]",
	"_numeric":     "numeric[]",
	"_uuid":        "uuid[]",
	"_json":        "json[]",
	"_jsonb":       "jsonb[]",
	"_bytea":       "bytea[]",
	"_inet":        "inet[]",
	"_cidr":        "cidr[]",
	"_macaddr":     "macaddr[]",
	"_macaddr8":    "macaddr8[]",
	"_date":        "date[]",
	"_time":        "time[]",
	"_timetz":      "timetz[]",
	"_timestamp":   "timestamp[]",
	"_timestamptz": "timestamptz[]",
	"_interval":    "interval[]",

	// Array types (basetype[] format from SQL query)
	"int2[]":        "smallint[]",
	"int4[]":        "integer[]",
	"int8[]":        "bigint[]",
	"float4[]":      "real[]",
	"float8[]":      "double precision[]",
	"bool[]":        "boolean[]",
	"varchar[]":     "varchar[]",
	"bpchar[]":      "character[]",
	"numeric[]":     "numeric[]",
	"uuid[]":        "uuid[]",
	"json[]":        "json[]",
	"jsonb[]":       "jsonb[]",
	"bytea[]":       "bytea[]",
	"inet[]":        "inet[]",
	"cidr[]":        "cidr[]",
	"macaddr[]":     "macaddr[]",
	"macaddr8[]":    "macaddr8[]",
	"date[]":        "date[]",
	"time[]":        "time[]",
	"timetz[]":      "timetz[]",
	"timestamp[]":   "timestamp[]",
	"timestamptz[]": "timestamptz[]",
	"interval[]":    "interval[]",

	// Other common types
	"pg_catalog.uuid":    "uuid",
	"pg_catalog.json":    "json",
	"pg_catalog.jsonb":   "jsonb",
	"pg_catalog.bytea":   "bytea",
	"pg_catalog.inet":    "inet",
	"pg_catalog.cidr":    "cidr",
	"pg_catalog.macaddr": "macaddr",

	// Serial types
	"serial":      "serial",
	"smallserial": "smallserial",
	"bigserial":   "bigserial",
}

// normalizePostgreSQLType normalizes PostgreSQL internal type names to standard SQL types.
// This function handles both expressions (with type casts) and direct type names.
func normalizePostgreSQLType(input string) string {
	if input == "" {
		return input
	}

	// Check if this is an expression with type casts (contains "::")
	if strings.Contains(input, "::") {
		// Handle expressions with type casts
		expr := input

		// Replace PostgreSQL internal type names with standard SQL types in type casts
		for pgType, sqlType := range postgresTypeNormalization {
			expr = strings.ReplaceAll(expr, "::"+pgType, "::"+sqlType)
		}

		// Handle pg_catalog prefix removal for unmapped types in type casts
		// Look for patterns like "::pg_catalog.sometype"
		if strings.Contains(expr, "::pg_catalog.") {
			expr = regexp.MustCompile(`::pg_catalog\.(\w+)`).ReplaceAllString(expr, "::$1")
		}

		return expr
	}

	// Handle direct type names
	typeName := input

	// Check if we have a direct mapping
	if normalized, exists := postgresTypeNormalization[typeName]; exists {
		return normalized
	}

	// Remove pg_catalog prefix for unmapped types
	if after, found := strings.CutPrefix(typeName, "pg_catalog."); found {
		return after
	}

	// Return as-is if no mapping found
	return typeName
}

// normalizeConstraint normalizes constraint definitions from inspector format to parser format
func normalizeConstraint(constraint *Constraint) {
	if constraint == nil {
		return
	}

	// Only normalize CHECK and EXCLUDE constraints - other constraint types are already consistent
	if constraint.Type == ConstraintTypeCheck && constraint.CheckClause != "" {
		constraint.CheckClause = normalizeCheckClause(constraint.CheckClause)
	}
	if constraint.Type == ConstraintTypeExclusion && constraint.ExclusionDefinition != "" {
		constraint.ExclusionDefinition = normalizeExclusionDefinition(constraint.ExclusionDefinition)
	}
}

// normalizeExclusionDefinition normalizes EXCLUDE constraint definitions.
//
// pg_get_constraintdef() returns the full definition like:
// "EXCLUDE USING gist (range_col WITH &&)"
// We keep it as-is since both desired and current state come from pg_get_constraintdef().
func normalizeExclusionDefinition(definition string) string {
	return strings.TrimSpace(definition)
}

// normalizeCheckClause normalizes CHECK constraint expressions.
//
// Since both desired state (from embedded postgres) and current state (from target database)
// now come from the same PostgreSQL version via pg_get_constraintdef(), they produce identical
// output. We only need basic cleanup for PostgreSQL internal representations.
func normalizeCheckClause(checkClause string) string {
	// Strip " NOT VALID" suffix if present (mimicking pg_dump behavior)
	// PostgreSQL's pg_get_constraintdef may include NOT VALID at the end,
	// but we want to control its placement via the IsValid field
	clause := strings.TrimSpace(checkClause)
	if strings.HasSuffix(clause, " NOT VALID") {
		clause = strings.TrimSuffix(clause, " NOT VALID")
		clause = strings.TrimSpace(clause)
	}

	// Remove "CHECK " prefix if present
	if after, found := strings.CutPrefix(clause, "CHECK "); found {
		clause = after
	}

	// Remove outer parentheses - pg_get_constraintdef wraps in parentheses
	clause = strings.TrimSpace(clause)
	if len(clause) > 0 && clause[0] == '(' && clause[len(clause)-1] == ')' {
		if isBalancedParentheses(clause[1 : len(clause)-1]) {
			clause = clause[1 : len(clause)-1]
			clause = strings.TrimSpace(clause)
		}
	}

	// Apply basic normalizations for PostgreSQL internal representations
	// (e.g., "~~ " to "LIKE", "= ANY (ARRAY[...])" to "IN (...)")
	normalizedClause := applyLegacyCheckNormalizations(clause)

	return fmt.Sprintf("CHECK (%s)", normalizedClause)
}

// applyLegacyCheckNormalizations applies the existing normalization patterns
func applyLegacyCheckNormalizations(clause string) string {
	// Normalize unnecessary parentheses around simple identifiers before type casts.
	// PostgreSQL sometimes stores "(column)::type" but the generated SQL uses "column::type".
	// These are semantically equivalent, so normalize to the simpler form for idempotency.
	// Pattern: (identifier)::type → identifier::type
	// Examples:
	//   - "(status)::text" → "status::text"
	//   - "((name))::varchar" → "name::varchar"
	parenCastRe := regexp.MustCompile(`\(([a-zA-Z_][a-zA-Z0-9_]*)\)::`)
	for {
		newClause := parenCastRe.ReplaceAllString(clause, `$1::`)
		if newClause == clause {
			break
		}
		clause = newClause
	}

	// Normalize redundant double type casts.
	// When pgschema generates CHECK (status::text IN ('value'::character varying, ...)),
	// PostgreSQL stores it with double casts: 'value'::character varying::text
	// But when the user writes CHECK (status IN ('value', ...)), PostgreSQL stores
	// just 'value'::character varying with ::text[] on the whole array.
	// Normalize the double cast to single cast for idempotent comparison.
	// Pattern: ::character varying::text → ::character varying
	// Pattern: ::varchar::text → ::varchar
	doubleCastRe := regexp.MustCompile(`::(character varying|varchar)::text\b`)
	clause = doubleCastRe.ReplaceAllString(clause, "::$1")

	// Convert PostgreSQL's "= ANY (ARRAY[...])" format to "IN (...)" format.
	// Type casts are preserved to maintain accuracy with PostgreSQL's internal representation.
	if strings.Contains(clause, "= ANY (ARRAY[") {
		return convertAnyArrayToIn(clause)
	}

	// Convert "column ~~ 'pattern'::text" to "column LIKE 'pattern'"
	if strings.Contains(clause, " ~~ ") {
		parts := strings.Split(clause, " ~~ ")
		if len(parts) == 2 {
			columnName := strings.TrimSpace(parts[0])
			pattern := strings.TrimSpace(parts[1])
			// Remove type cast
			if idx := strings.Index(pattern, "::"); idx != -1 {
				pattern = pattern[:idx]
			}
			return fmt.Sprintf("%s LIKE %s", columnName, pattern)
		}
	}

	return clause
}

// builtInTypes is a set of PostgreSQL built-in type names (both internal and standard forms)
// Used by IsBuiltInType to determine if a type requires special handling during migrations
var builtInTypes = map[string]bool{
	// Numeric types
	"smallint": true, "int2": true,
	"integer": true, "int": true, "int4": true,
	"bigint": true, "int8": true,
	"decimal": true, "numeric": true,
	"real": true, "float4": true,
	"double precision": true, "float8": true,
	"smallserial": true, "serial2": true,
	"serial": true, "serial4": true,
	"bigserial": true, "serial8": true,
	"money": true,

	// Character types
	"character varying": true, "varchar": true,
	"character": true, "char": true, "bpchar": true,
	"text": true,
	"name": true,

	// Binary types
	"bytea": true,

	// Date/Time types
	"timestamp": true, "timestamp without time zone": true,
	"timestamptz": true, "timestamp with time zone": true,
	"date": true,
	"time": true, "time without time zone": true,
	"timetz": true, "time with time zone": true,
	"interval": true,

	// Boolean type
	"boolean": true, "bool": true,

	// Geometric types
	"point": true, "line": true, "lseg": true, "box": true,
	"path": true, "polygon": true, "circle": true,

	// Network address types
	"cidr": true, "inet": true, "macaddr": true, "macaddr8": true,

	// Bit string types
	"bit": true, "bit varying": true, "varbit": true,

	// Text search types
	"tsvector": true, "tsquery": true,

	// UUID type
	"uuid": true,

	// XML type
	"xml": true,

	// JSON types
	"json": true, "jsonb": true,

	// Range types
	"int4range": true, "int8range": true, "numrange": true,
	"tsrange": true, "tstzrange": true, "daterange": true,

	// OID types
	"oid": true, "regproc": true, "regprocedure": true,
	"regoper": true, "regoperator": true, "regclass": true,
	"regtype": true, "regrole": true, "regnamespace": true,
	"regconfig": true, "regdictionary": true,
}

// IsBuiltInType checks if a type name is a PostgreSQL built-in type.
// This is used to determine if type conversions need explicit USING clauses
// (e.g., text -> custom_enum requires USING, but integer -> bigint does not)
func IsBuiltInType(typeName string) bool {
	if typeName == "" {
		return false
	}

	// Normalize the type name for comparison
	t := strings.ToLower(typeName)

	// Strip array suffix if present
	t = strings.TrimSuffix(t, "[]")

	// Extract base type name (handle types like varchar(255), numeric(10,2))
	if idx := strings.Index(t, "("); idx != -1 {
		t = t[:idx]
	}

	// Strip pg_catalog prefix if present
	t = strings.TrimPrefix(t, "pg_catalog.")

	return builtInTypes[t]
}

// IsTextLikeType checks if a type is a text-like type (text, varchar, char, etc.)
// This is used to determine if type conversions need explicit USING clauses
func IsTextLikeType(typeName string) bool {
	// Normalize the type name for comparison
	t := strings.ToLower(typeName)

	// Strip array suffix if present
	t = strings.TrimSuffix(t, "[]")

	// Check for text-like types
	switch {
	case t == "text":
		return true
	case t == "varchar" || strings.HasPrefix(t, "varchar(") || t == "character varying" || strings.HasPrefix(t, "character varying("):
		return true
	case t == "char" || strings.HasPrefix(t, "char(") || t == "character" || strings.HasPrefix(t, "character("):
		return true
	case t == "bpchar":
		return true
	}

	return false
}

// convertAnyArrayToIn converts PostgreSQL's "column = ANY (ARRAY[...])" format
// to the more readable "column IN (...)" format.
//
// Type casts are always preserved to ensure:
// - Custom types (enums, domains) are properly qualified (e.g., 'value'::public.my_enum)
// - Output matches pg_dump's format exactly
// - Comparison between desired and current states is accurate
//
// Example transformations:
//   - "status = ANY (ARRAY['active'::public.status_type])" → "status IN ('active'::public.status_type)"
//   - "gender = ANY (ARRAY['M'::text, 'F'::text])" → "gender IN ('M'::text, 'F'::text)"
//   - "(col = ANY (ARRAY[...])) AND (other)" → "(col IN (...)) AND (other)"
//   - "col::text = ANY (ARRAY['a'::varchar]::text[])" → "col::text IN ('a'::varchar)"
func convertAnyArrayToIn(expr string) string {
	const marker = " = ANY (ARRAY["
	idx := strings.Index(expr, marker)
	if idx == -1 {
		return expr
	}

	// Extract the part before the marker (column name with possible leading content)
	prefix := expr[:idx]

	// Find the closing "]" for ARRAY[...] starting after the marker
	startIdx := idx + len(marker)
	arrayEnd := findArrayClose(expr, startIdx)
	if arrayEnd == -1 {
		return expr
	}

	// Extract array contents
	arrayContents := expr[startIdx:arrayEnd]

	// Find the closing ")" for ANY(...), which may be after an optional type cast like "::text[]"
	// Pattern after "]": optional "::type[]" followed by ")"
	closeParenIdx := arrayEnd + 1
	for closeParenIdx < len(expr) && expr[closeParenIdx] != ')' {
		closeParenIdx++
	}
	if closeParenIdx >= len(expr) {
		return expr // No closing paren found
	}

	// Everything after the closing ")" is the suffix
	suffix := expr[closeParenIdx+1:]

	// Split values and preserve them as-is, including all type casts
	values := strings.Split(arrayContents, ", ")
	var cleanValues []string
	for _, val := range values {
		val = strings.TrimSpace(val)
		cleanValues = append(cleanValues, val)
	}

	// Return converted format: "prefix IN (values)suffix"
	return fmt.Sprintf("%s IN (%s)%s", prefix, strings.Join(cleanValues, ", "), suffix)
}

// findArrayClose finds the position of the closing "]" for an ARRAY literal,
// handling nested brackets and quoted strings properly.
// Returns the position of the "]" that closes the ARRAY.
func findArrayClose(expr string, startIdx int) int {
	bracketDepth := 1 // We're already inside ARRAY[
	inQuote := false

	for i := startIdx; i < len(expr); i++ {
		ch := expr[i]

		if inQuote {
			if ch == '\'' {
				// Check for escaped quote ''
				if i+1 < len(expr) && expr[i+1] == '\'' {
					i++ // Skip escaped quote
					continue
				}
				inQuote = false
			}
			continue
		}

		switch ch {
		case '\'':
			inQuote = true
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
			if bracketDepth == 0 {
				// Found the closing ] for the ARRAY
				return i
			}
		}
	}

	return -1 // Not found
}

