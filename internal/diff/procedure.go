package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateProceduresSQL generates CREATE PROCEDURE statements
func generateCreateProceduresSQL(procedures []*ir.Procedure, targetSchema string, collector *diffCollector) {
	// Sort procedures by name for consistent ordering
	sortedProcedures := make([]*ir.Procedure, len(procedures))
	copy(sortedProcedures, procedures)
	sort.Slice(sortedProcedures, func(i, j int) bool {
		return sortedProcedures[i].Name < sortedProcedures[j].Name
	})

	for _, procedure := range sortedProcedures {
		sql := generateProcedureSQL(procedure, targetSchema)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeProcedure,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", procedure.Schema, procedure.Name),
			Source:              procedure,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)

		// Generate COMMENT ON PROCEDURE if the procedure has a comment
		if procedure.Comment != "" {
			generateProcedureComment(procedure, targetSchema, DiffTypeProcedure, DiffOperationCreate, collector)
		}
	}
}

// generateModifyProceduresSQL generates DROP and CREATE PROCEDURE statements for modified procedures
func generateModifyProceduresSQL(diffs []*procedureDiff, targetSchema string, collector *diffCollector) {
	for _, diff := range diffs {
		oldProc := diff.Old
		newProc := diff.New

		// Check if only comment changed (no body changes)
		onlyCommentChanged := proceduresEqualExceptComment(oldProc, newProc) && oldProc.Comment != newProc.Comment

		if onlyCommentChanged {
			// Only the comment changed - generate just COMMENT ON PROCEDURE
			generateProcedureComment(newProc, targetSchema, DiffTypeProcedure, DiffOperationAlter, collector)
			continue
		}

		// Drop the old procedure first
		procedureName := qualifyEntityName(oldProc.Schema, oldProc.Name, targetSchema)
		var dropSQL string

		// For DROP statements, we need the full parameter signature including modes and names
		paramSignature := formatProcedureParametersForDrop(oldProc)
		if paramSignature != "" {
			dropSQL = fmt.Sprintf("DROP PROCEDURE IF EXISTS %s(%s);", procedureName, paramSignature)
		} else {
			dropSQL = fmt.Sprintf("DROP PROCEDURE IF EXISTS %s();", procedureName)
		}

		// Create the new procedure
		createSQL := generateProcedureSQL(newProc, targetSchema)

		// Create a single context with ALTER operation and multiple statements
		// This represents the modification as a single operation in the summary
		alterContext := &diffContext{
			Type:                DiffTypeProcedure,
			Operation:           DiffOperationAlter,
			Path:                fmt.Sprintf("%s.%s", newProc.Schema, newProc.Name),
			Source:              diff,
			CanRunInTransaction: true,
		}

		// Collect both DROP and CREATE as separate statements within a single diff
		statements := []SQLStatement{
			{SQL: dropSQL, CanRunInTransaction: true},
			{SQL: createSQL, CanRunInTransaction: true},
		}

		collector.collectStatements(alterContext, statements)

		// Check if comment also changed alongside body changes
		if oldProc.Comment != newProc.Comment {
			generateProcedureComment(newProc, targetSchema, DiffTypeProcedure, DiffOperationAlter, collector)
		}
	}
}

// generateDropProceduresSQL generates DROP PROCEDURE statements
func generateDropProceduresSQL(procedures []*ir.Procedure, targetSchema string, collector *diffCollector) {
	// Sort procedures by name for consistent ordering
	sortedProcedures := make([]*ir.Procedure, len(procedures))
	copy(sortedProcedures, procedures)
	sort.Slice(sortedProcedures, func(i, j int) bool {
		return sortedProcedures[i].Name < sortedProcedures[j].Name
	})

	for _, procedure := range sortedProcedures {
		procedureName := qualifyEntityName(procedure.Schema, procedure.Name, targetSchema)
		var sql string

		// For DROP statements, we need the full parameter signature including modes and names
		// Extract the complete signature from the procedure
		paramSignature := formatProcedureParametersForDrop(procedure)
		if paramSignature != "" {
			sql = fmt.Sprintf("DROP PROCEDURE IF EXISTS %s(%s);", procedureName, paramSignature)
		} else {
			sql = fmt.Sprintf("DROP PROCEDURE IF EXISTS %s();", procedureName)
		}

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeProcedure,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", procedure.Schema, procedure.Name),
			Source:              procedure,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// formatParameterString formats a single parameter with mode, name, type, and optional default value
// includeDefault controls whether DEFAULT clauses are included in the output
func formatParameterString(param *ir.Parameter, includeDefault bool, targetSchema string) string {
	var part string
	// Always include mode for clarity (IN is default but we make it explicit)
	if param.Mode != "" {
		part = param.Mode + " "
	} else {
		part = "IN "
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
	if includeDefault && param.DefaultValue != nil {
		part += " DEFAULT " + *param.DefaultValue
	}
	return part
}

// generateProcedureSQL generates CREATE OR REPLACE PROCEDURE SQL for a procedure
func generateProcedureSQL(procedure *ir.Procedure, targetSchema string) string {
	var stmt strings.Builder

	// Build the CREATE OR REPLACE PROCEDURE header with schema qualification
	procedureName := qualifyEntityName(procedure.Schema, procedure.Name, targetSchema)
	stmt.WriteString(fmt.Sprintf("CREATE OR REPLACE PROCEDURE %s", procedureName))

	// Add parameters from structured Parameters array
	// Always include mode explicitly (matching pg_dump behavior)
	var paramParts []string
	for _, param := range procedure.Parameters {
		paramParts = append(paramParts, formatParameterString(param, true, targetSchema))
	}
	if len(paramParts) > 0 {
		stmt.WriteString(fmt.Sprintf("(\n    %s\n)", strings.Join(paramParts, ",\n    ")))
	} else {
		stmt.WriteString("()")
	}

	// Add language
	if procedure.Language != "" {
		stmt.WriteString(fmt.Sprintf("\nLANGUAGE %s", procedure.Language))
	}

	// Note: Procedures don't have SECURITY DEFINER/INVOKER in PostgreSQL
	// This is a function-only feature

	// Add the procedure body
	if procedure.Definition != "" {
		// Check if this uses SQL-standard body syntax (PG14+)
		// pg_get_function_sqlbody returns "BEGIN ATOMIC ... END" for SQL-standard procedure bodies
		// These should not be wrapped with AS $$ ... $$
		// Note: The RETURN check is kept for consistency with function handling,
		// though procedures don't support value-returning RETURN statements
		trimmedDef := strings.TrimSpace(procedure.Definition)
		isSQLStandardBody := (len(trimmedDef) >= 7 && strings.EqualFold(trimmedDef[:7], "RETURN ")) ||
			(len(trimmedDef) >= 12 && strings.EqualFold(trimmedDef[:12], "BEGIN ATOMIC"))
		if isSQLStandardBody {
			stmt.WriteString(fmt.Sprintf("\n%s;", trimmedDef))
		} else {
			// Traditional AS $$ ... $$ syntax
			tag := generateProcedureDollarQuoteTag(procedure.Definition)
			stmt.WriteString(fmt.Sprintf("\nAS %s%s%s;", tag, procedure.Definition, tag))
		}
	} else {
		stmt.WriteString("\nAS $$$$;")
	}

	return stmt.String()
}

// generateProcedureDollarQuoteTag creates a safe dollar quote tag that doesn't conflict with the procedure body content.
// This implements the same algorithm used by pg_dump to avoid conflicts.
func generateProcedureDollarQuoteTag(body string) string {
	// Check if the body contains potential conflicts with $$ quoting:
	// 1. Direct $$ sequences
	// 2. Parameter references like $1, $2, etc. that could be ambiguous
	needsTagged := strings.Contains(body, "$$") || containsProcedureParameterReferences(body)

	if !needsTagged {
		return "$$"
	}

	// Start with the pg_dump preferred tag
	candidates := []string{"$_$", "$procedure$", "$body$", "$pgdump$"}

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

// containsProcedureParameterReferences checks if the body contains PostgreSQL parameter references ($1, $2, etc.)
// that could be confused with dollar quoting delimiters
func containsProcedureParameterReferences(body string) bool {
	// Simple check for $digit patterns which are PostgreSQL parameter references
	for i := 0; i < len(body)-1; i++ {
		if body[i] == '$' && i+1 < len(body) && body[i+1] >= '0' && body[i+1] <= '9' {
			return true
		}
	}
	return false
}

// proceduresEqual compares two procedures for equality
func proceduresEqual(old, new *ir.Procedure) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if !definitionsEqualIgnoringSchema(old.Definition, new.Definition, old.Schema) {
		return false
	}
	if old.Language != new.Language {
		return false
	}
	if old.Comment != new.Comment {
		return false
	}

	// Compare using normalized Parameters array instead of Signature
	// This ensures proper comparison regardless of how parameters are specified
	hasOldParams := len(old.Parameters) > 0
	hasNewParams := len(new.Parameters) > 0

	if hasOldParams && hasNewParams {
		// Both have Parameters - compare them
		return parametersEqual(old.Parameters, new.Parameters)
	} else if hasOldParams || hasNewParams {
		// One has Parameters, one doesn't - they're different
		return false
	}

	// Both have no parameters - they're equal
	return true
}

// proceduresEqualExceptComment compares two procedures ignoring comment differences
// Used to determine if only the comment changed (no body changes needed)
func proceduresEqualExceptComment(old, new *ir.Procedure) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if !definitionsEqualIgnoringSchema(old.Definition, new.Definition, old.Schema) {
		return false
	}
	if old.Language != new.Language {
		return false
	}
	// Note: We intentionally do NOT compare Comment here

	hasOldParams := len(old.Parameters) > 0
	hasNewParams := len(new.Parameters) > 0

	if hasOldParams && hasNewParams {
		return parametersEqual(old.Parameters, new.Parameters)
	} else if hasOldParams || hasNewParams {
		return false
	}

	return true
}

// formatProcedureParametersForDrop formats procedure parameters for DROP PROCEDURE statements
// Returns the full parameter signature including mode and name (e.g., "IN order_id integer, IN amount numeric")
// This is necessary for proper procedure identification in PostgreSQL
func formatProcedureParametersForDrop(procedure *ir.Procedure) string {
	// Use the structured Parameters array
	var paramParts []string
	for _, param := range procedure.Parameters {
		// Use helper function with includeDefault=false for DROP statements
		// Pass empty targetSchema since DROP statements use full qualified names
		paramParts = append(paramParts, formatParameterString(param, false, ""))
	}
	return strings.Join(paramParts, ", ")
}

// generateProcedureComment generates COMMENT ON PROCEDURE statement
func generateProcedureComment(
	procedure *ir.Procedure,
	targetSchema string,
	diffType DiffType,
	operation DiffOperation,
	collector *diffCollector,
) {
	procedureName := qualifyEntityName(procedure.Schema, procedure.Name, targetSchema)
	argsList := procedure.GetArguments()

	var sql string
	if procedure.Comment == "" {
		sql = fmt.Sprintf("COMMENT ON PROCEDURE %s(%s) IS NULL;", procedureName, argsList)
	} else {
		sql = fmt.Sprintf("COMMENT ON PROCEDURE %s(%s) IS %s;", procedureName, argsList, quoteString(procedure.Comment))
	}

	context := &diffContext{
		Type:                diffType,
		Operation:           operation,
		Path:                fmt.Sprintf("%s.%s", procedure.Schema, procedure.Name),
		Source:              procedure,
		CanRunInTransaction: true,
	}
	collector.collect(context, sql)
}
