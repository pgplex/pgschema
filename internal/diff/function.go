package diff

import (
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateFunctionsSQL generates CREATE FUNCTION statements
func generateCreateFunctionsSQL(functions []*ir.Function, targetSchema string, collector *diffCollector) {
	// Build dependencies from function bodies (supplements pg_depend, which doesn't track SQL function body references)
	buildFunctionBodyDependencies(functions)

	// Sort functions by dependency order (topological sort)
	sortedFunctions := topologicallySortFunctions(functions)

	for _, function := range sortedFunctions {
		sql := generateFunctionSQL(function, targetSchema)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeFunction,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", function.Schema, function.Name),
			Source:              function,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)

		// Generate COMMENT ON FUNCTION if the function has a comment
		if function.Comment != "" {
			generateFunctionComment(function, targetSchema, DiffTypeFunction, DiffOperationCreate, collector)
		}
	}
}

// generateModifyFunctionsSQL generates ALTER FUNCTION statements
func generateModifyFunctionsSQL(diffs []*functionDiff, targetSchema string, collector *diffCollector) {
	for _, diff := range diffs {
		oldFunc := diff.Old
		newFunc := diff.New

		// Check if only comment changed (no body/attribute changes)
		onlyCommentChanged := functionsEqualExceptComment(oldFunc, newFunc) && oldFunc.Comment != newFunc.Comment

		if onlyCommentChanged {
			// Only the comment changed - generate just COMMENT ON FUNCTION
			generateFunctionComment(newFunc, targetSchema, DiffTypeFunction, DiffOperationAlter, collector)
			continue
		}

		// Check if only LEAKPROOF or PARALLEL attributes changed (not the function body/definition)
		onlyAttributesChanged := functionsEqualExceptAttributes(oldFunc, newFunc)

		if onlyAttributesChanged {
			// Generate ALTER FUNCTION statements for attribute-only changes
			// Check PARALLEL changes
			if oldFunc.Parallel != newFunc.Parallel {
				stmt := fmt.Sprintf("ALTER FUNCTION %s(%s) PARALLEL %s;",
					qualifyEntityName(newFunc.Schema, newFunc.Name, targetSchema),
					newFunc.GetArguments(),
					newFunc.Parallel)

				context := &diffContext{
					Type:                DiffTypeFunction,
					Operation:           DiffOperationAlter,
					Path:                fmt.Sprintf("%s.%s", newFunc.Schema, newFunc.Name),
					Source:              diff,
					CanRunInTransaction: true,
				}
				collector.collect(context, stmt)
			}

			// Check LEAKPROOF changes
			if oldFunc.IsLeakproof != newFunc.IsLeakproof {
				var stmt string
				if newFunc.IsLeakproof {
					stmt = fmt.Sprintf("ALTER FUNCTION %s(%s) LEAKPROOF;",
						qualifyEntityName(newFunc.Schema, newFunc.Name, targetSchema),
						newFunc.GetArguments())
				} else {
					stmt = fmt.Sprintf("ALTER FUNCTION %s(%s) NOT LEAKPROOF;",
						qualifyEntityName(newFunc.Schema, newFunc.Name, targetSchema),
						newFunc.GetArguments())
				}

				context := &diffContext{
					Type:                DiffTypeFunction,
					Operation:           DiffOperationAlter,
					Path:                fmt.Sprintf("%s.%s", newFunc.Schema, newFunc.Name),
					Source:              diff,
					CanRunInTransaction: true,
				}
				collector.collect(context, stmt)
			}

			// Check if comment also changed alongside attributes
			if oldFunc.Comment != newFunc.Comment {
				generateFunctionComment(newFunc, targetSchema, DiffTypeFunction, DiffOperationAlter, collector)
			}
		} else if functionRequiresRecreate(oldFunc, newFunc) {
			// Return type, OUT parameters, or parameter names changed - must DROP then CREATE
			// PostgreSQL does not allow CREATE OR REPLACE to change these.
			// See https://github.com/pgplex/pgschema/issues/326
			dropSQL := generateDropFunctionSQL(oldFunc, targetSchema)
			createSQL := generateFunctionSQL(newFunc, targetSchema)

			alterContext := &diffContext{
				Type:                DiffTypeFunction,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", newFunc.Schema, newFunc.Name),
				Source:              diff,
				CanRunInTransaction: true,
			}

			statements := []SQLStatement{
				{SQL: dropSQL, CanRunInTransaction: true},
				{SQL: createSQL, CanRunInTransaction: true},
			}
			collector.collectStatements(alterContext, statements)

			// Check if comment also changed alongside body changes
			if oldFunc.Comment != newFunc.Comment {
				generateFunctionComment(newFunc, targetSchema, DiffTypeFunction, DiffOperationAlter, collector)
			}
		} else {
			// Function body or other attributes changed - use CREATE OR REPLACE
			sql := generateFunctionSQL(newFunc, targetSchema)

			// Create context for this statement
			context := &diffContext{
				Type:                DiffTypeFunction,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", newFunc.Schema, newFunc.Name),
				Source:              diff,
				CanRunInTransaction: true,
			}

			collector.collect(context, sql)

			// Check if comment also changed alongside body changes
			if oldFunc.Comment != newFunc.Comment {
				generateFunctionComment(newFunc, targetSchema, DiffTypeFunction, DiffOperationAlter, collector)
			}
		}
	}
}

// generateDropFunctionsSQL generates DROP FUNCTION statements
func generateDropFunctionsSQL(functions []*ir.Function, targetSchema string, collector *diffCollector) {
	// Sort functions by reverse dependency order (drop dependents before dependencies)
	sortedFunctions := reverseSlice(topologicallySortFunctions(functions))

	for _, function := range sortedFunctions {
		sql := generateDropFunctionSQL(function, targetSchema)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeFunction,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", function.Schema, function.Name),
			Source:              function,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateDropFunctionSQL generates a DROP FUNCTION IF EXISTS statement
func generateDropFunctionSQL(function *ir.Function, targetSchema string) string {
	functionName := qualifyEntityName(function.Schema, function.Name, targetSchema)
	argsList := function.GetArguments()
	if argsList != "" {
		return fmt.Sprintf("DROP FUNCTION IF EXISTS %s(%s);", functionName, argsList)
	}
	return fmt.Sprintf("DROP FUNCTION IF EXISTS %s();", functionName)
}

// generateFunctionSQL generates CREATE OR REPLACE FUNCTION SQL for a function
func generateFunctionSQL(function *ir.Function, targetSchema string) string {
	var stmt strings.Builder

	// Build the CREATE OR REPLACE FUNCTION header with schema qualification
	functionName := qualifyEntityName(function.Schema, function.Name, targetSchema)
	stmt.WriteString(fmt.Sprintf("CREATE OR REPLACE FUNCTION %s", functionName))

	// Add parameters from structured Parameters array
	// Exclude TABLE mode parameters as they're part of RETURNS clause
	var paramParts []string
	for _, param := range function.Parameters {
		if param.Mode != "TABLE" {
			paramParts = append(paramParts, formatFunctionParameter(param, true, targetSchema))
		}
	}
	if len(paramParts) > 0 {
		stmt.WriteString(fmt.Sprintf("(\n    %s\n)", strings.Join(paramParts, ",\n    ")))
	} else {
		stmt.WriteString("()")
	}

	// Add return type
	if function.ReturnType != "" {
		// Strip schema prefix from return type if it matches the target schema
		returnType := stripSchemaPrefix(function.ReturnType, targetSchema)
		stmt.WriteString(fmt.Sprintf("\nRETURNS %s", returnType))
	}

	// Add language
	if function.Language != "" {
		stmt.WriteString(fmt.Sprintf("\nLANGUAGE %s", function.Language))
	}

	// Add volatility if not default
	if function.Volatility != "" {
		stmt.WriteString(fmt.Sprintf("\n%s", function.Volatility))
	}

	// Add STRICT if specified
	if function.IsStrict {
		stmt.WriteString("\nSTRICT")
	}

	// Add SECURITY DEFINER if true (INVOKER is default and not output)
	if function.IsSecurityDefiner {
		stmt.WriteString("\nSECURITY DEFINER")
	}

	// Add LEAKPROOF if true
	if function.IsLeakproof {
		stmt.WriteString("\nLEAKPROOF")
	}
	// Note: Don't output NOT LEAKPROOF (it's the default)

	// Add PARALLEL if not default (UNSAFE)
	if function.Parallel == "SAFE" {
		stmt.WriteString("\nPARALLEL SAFE")
	} else if function.Parallel == "RESTRICTED" {
		stmt.WriteString("\nPARALLEL RESTRICTED")
	}
	// Note: Don't output PARALLEL UNSAFE (it's the default)

	// Add SET search_path if specified
	// Note: Multi-schema paths are output unquoted (e.g., "SET search_path = pg_catalog, public"),
	// except for the empty search_path case which requires single quotes: SET search_path = ''
	if function.SearchPath != "" {
		// PostgreSQL stores SET search_path = '' as search_path="" in proconfig.
		// The extracted value is "" (two double-quote chars). Render as '' (single-quoted empty string).
		// Only the whole-value empty case is handled; mixed paths (e.g. pg_catalog, "") are not expected.
		if function.SearchPath == `""` {
			stmt.WriteString("\nSET search_path = ''")
		} else {
			stmt.WriteString(fmt.Sprintf("\nSET search_path = %s", function.SearchPath))
		}
	}

	// Add the function body
	if function.Definition != "" {
		// Check if this uses SQL-standard body syntax (PG14+)
		// pg_get_function_sqlbody returns:
		// - "RETURN expression" for simple SQL-standard bodies
		// - "BEGIN ATOMIC ... END" for multi-statement SQL-standard bodies
		// These should not be wrapped with AS $$ ... $$
		trimmedDef := strings.TrimSpace(function.Definition)
		isSQLStandardBody := (len(trimmedDef) >= 7 && strings.EqualFold(trimmedDef[:7], "RETURN ")) ||
			(len(trimmedDef) >= 12 && strings.EqualFold(trimmedDef[:12], "BEGIN ATOMIC"))
		if isSQLStandardBody {
			stmt.WriteString(fmt.Sprintf("\n%s;", trimmedDef))
		} else {
			// Traditional AS $$ ... $$ syntax
			tag := generateDollarQuoteTag(function.Definition)
			stmt.WriteString(fmt.Sprintf("\nAS %s%s%s;", tag, function.Definition, tag))
		}
	} else {
		stmt.WriteString("\nAS $$$$;")
	}

	return stmt.String()
}

// generateDollarQuoteTag creates a safe dollar quote tag that doesn't conflict with the function body content.
// This implements the same algorithm used by pg_dump to avoid conflicts.
func generateDollarQuoteTag(body string) string {
	// Check if the body contains potential conflicts with $$ quoting:
	// 1. Direct $$ sequences
	// 2. Parameter references like $1, $2, etc. that could be ambiguous
	needsTagged := strings.Contains(body, "$$") || containsParameterReferences(body)

	if !needsTagged {
		return "$$"
	}

	// Start with the pg_dump preferred tag
	candidates := []string{"$_$", "$function$", "$body$", "$pgdump$"}

	// Try each candidate tag
	for _, tag := range candidates {
		if !strings.Contains(body, tag) {
			return tag
		}
	}

	// If all predefined tags conflict, generate a unique one
	// Use a simple incrementing number approach like pg_dump does
	for i := 1; i < 1000; i++ {
		tag := fmt.Sprintf("$tag%d$", i)
		if !strings.Contains(body, tag) {
			return tag
		}
	}

	// Fallback - this should rarely happen
	return "$fallback$"
}

// containsParameterReferences checks if the body contains PostgreSQL parameter references ($1, $2, etc.)
// that could be confused with dollar quoting delimiters
func containsParameterReferences(body string) bool {
	// Simple check for $digit patterns which are PostgreSQL parameter references
	for i := 0; i < len(body)-1; i++ {
		if body[i] == '$' && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '9' {
			return true
		}
	}
	return false
}

// formatFunctionParameter formats a single function parameter with name, type, and optional default value
// For functions, mode is typically omitted (unlike procedures) unless it's OUT/INOUT
// includeDefault controls whether DEFAULT clauses are included in the output
func formatFunctionParameter(param *ir.Parameter, includeDefault bool, targetSchema string) string {
	var part string

	// For functions, only include mode if it's OUT or INOUT (IN is implicit)
	if param.Mode == "OUT" || param.Mode == "INOUT" || param.Mode == "VARIADIC" {
		part = param.Mode + " "
	}

	// Add parameter name and type
	// Strip schema prefix from data type if it matches the target schema
	dataType := stripSchemaPrefix(param.DataType, targetSchema)
	if param.Name != "" {
		part += param.Name + " " + dataType
	} else {
		part += dataType
	}

	// Add DEFAULT value if present and requested
	// Strip schema prefix from default value type casts
	// We strip both the target schema prefix and any temporary schema prefix (pgschema_tmp_*)
	if includeDefault && param.DefaultValue != nil {
		defaultVal := *param.DefaultValue
		// Strip target schema prefix
		defaultVal = stripSchemaPrefix(defaultVal, targetSchema)
		// Also strip temporary embedded postgres schema prefixes (pgschema_tmp_*)
		defaultVal = stripTempSchemaPrefix(defaultVal)
		part += " DEFAULT " + defaultVal
	}

	return part
}

// functionsEqualExceptAttributes compares two functions ignoring LEAKPROOF and PARALLEL attributes
// Used to determine if ALTER FUNCTION can be used instead of CREATE OR REPLACE
func functionsEqualExceptAttributes(old, new *ir.Function) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if old.Definition != new.Definition {
		return false
	}
	if old.ReturnType != new.ReturnType {
		return false
	}
	if old.Language != new.Language {
		return false
	}
	if old.Volatility != new.Volatility {
		return false
	}
	if old.IsStrict != new.IsStrict {
		return false
	}
	if old.IsSecurityDefiner != new.IsSecurityDefiner {
		return false
	}
	if old.SearchPath != new.SearchPath {
		return false
	}
	// Note: We intentionally do NOT compare IsLeakproof or Parallel here
	// That's the whole point - we want to detect when only those attributes changed

	// Compare using normalized Parameters array
	oldInputParams := filterNonTableParameters(old.Parameters)
	newInputParams := filterNonTableParameters(new.Parameters)
	return parametersEqual(oldInputParams, newInputParams)
}

// functionsEqual compares two functions for equality
func functionsEqual(old, new *ir.Function) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if old.Definition != new.Definition {
		return false
	}
	if old.ReturnType != new.ReturnType {
		return false
	}
	if old.Language != new.Language {
		return false
	}
	if old.Volatility != new.Volatility {
		return false
	}
	if old.IsStrict != new.IsStrict {
		return false
	}
	if old.IsSecurityDefiner != new.IsSecurityDefiner {
		return false
	}
	if old.IsLeakproof != new.IsLeakproof {
		return false
	}
	if old.Parallel != new.Parallel {
		return false
	}
	if old.SearchPath != new.SearchPath {
		return false
	}
	if old.Comment != new.Comment {
		return false
	}

	// Compare using normalized Parameters array
	// This ensures type aliases like "character varying" vs "varchar" are treated as equal
	// For RETURNS TABLE functions, exclude TABLE mode parameters (they're in ReturnType)
	// Only compare input parameters (IN, INOUT, VARIADIC, OUT)
	oldInputParams := filterNonTableParameters(old.Parameters)
	newInputParams := filterNonTableParameters(new.Parameters)
	return parametersEqual(oldInputParams, newInputParams)
}

// functionsEqualExceptComment compares two functions ignoring comment differences
// Used to determine if only the comment changed (no body/attribute changes needed)
func functionsEqualExceptComment(old, new *ir.Function) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if old.Definition != new.Definition {
		return false
	}
	if old.ReturnType != new.ReturnType {
		return false
	}
	if old.Language != new.Language {
		return false
	}
	if old.Volatility != new.Volatility {
		return false
	}
	if old.IsStrict != new.IsStrict {
		return false
	}
	if old.IsSecurityDefiner != new.IsSecurityDefiner {
		return false
	}
	if old.IsLeakproof != new.IsLeakproof {
		return false
	}
	if old.Parallel != new.Parallel {
		return false
	}
	if old.SearchPath != new.SearchPath {
		return false
	}
	// Note: We intentionally do NOT compare Comment here

	oldInputParams := filterNonTableParameters(old.Parameters)
	newInputParams := filterNonTableParameters(new.Parameters)
	return parametersEqual(oldInputParams, newInputParams)
}

// functionRequiresRecreate checks if a function modification requires DROP+CREATE
// instead of CREATE OR REPLACE. PostgreSQL does not allow CREATE OR REPLACE to change
// the return type or parameter names of an existing function.
func functionRequiresRecreate(old, new *ir.Function) bool {
	if old.ReturnType != new.ReturnType {
		return true
	}
	// Check parameter changes that CREATE OR REPLACE cannot handle.
	// Input parameter types are the same (same map key), but names, OUT/INOUT
	// parameter types/modes, or parameter count differences require DROP+CREATE.
	oldParams := filterNonTableParameters(old.Parameters)
	newParams := filterNonTableParameters(new.Parameters)
	if len(oldParams) != len(newParams) {
		return true
	}
	for i := range oldParams {
		if oldParams[i].Name != newParams[i].Name {
			return true
		}
		// OUT/INOUT parameter type or mode changes also require DROP+CREATE
		if oldParams[i].Mode == "OUT" || oldParams[i].Mode == "INOUT" ||
			newParams[i].Mode == "OUT" || newParams[i].Mode == "INOUT" {
			if !parameterEqual(oldParams[i], newParams[i]) {
				return true
			}
		}
	}
	return false
}

// filterNonTableParameters filters out TABLE mode parameters
// TABLE parameters are output columns in RETURNS TABLE() and shouldn't be compared as input parameters
func filterNonTableParameters(params []*ir.Parameter) []*ir.Parameter {
	var filtered []*ir.Parameter
	for _, param := range params {
		if param.Mode != "TABLE" {
			filtered = append(filtered, param)
		}
	}
	return filtered
}

// parametersEqual compares two parameter arrays for equality
func parametersEqual(oldParams, newParams []*ir.Parameter) bool {
	if len(oldParams) != len(newParams) {
		return false
	}

	for i := range oldParams {
		if !parameterEqual(oldParams[i], newParams[i]) {
			return false
		}
	}

	return true
}

// parameterEqual compares two parameters for equality
func parameterEqual(old, new *ir.Parameter) bool {
	if old.Name != new.Name {
		return false
	}

	// Compare data types (already normalized by ir.normalizeFunction)
	if old.DataType != new.DataType {
		return false
	}

	if old.Mode != new.Mode {
		return false
	}

	// Compare default values
	if (old.DefaultValue == nil) != (new.DefaultValue == nil) {
		return false
	}
	if old.DefaultValue != nil && new.DefaultValue != nil {
		if *old.DefaultValue != *new.DefaultValue {
			return false
		}
	}

	return true
}

// generateFunctionComment generates COMMENT ON FUNCTION statement
func generateFunctionComment(
	function *ir.Function,
	targetSchema string,
	diffType DiffType,
	operation DiffOperation,
	collector *diffCollector,
) {
	functionName := qualifyEntityName(function.Schema, function.Name, targetSchema)
	argsList := function.GetArguments()

	var sql string
	if function.Comment == "" {
		sql = fmt.Sprintf("COMMENT ON FUNCTION %s(%s) IS NULL;", functionName, argsList)
	} else {
		sql = fmt.Sprintf("COMMENT ON FUNCTION %s(%s) IS %s;", functionName, argsList, quoteString(function.Comment))
	}

	context := &diffContext{
		Type:                diffType,
		Operation:           operation,
		Path:                fmt.Sprintf("%s.%s", function.Schema, function.Name),
		Source:              function,
		CanRunInTransaction: true,
	}
	collector.collect(context, sql)
}
