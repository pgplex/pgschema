package dump

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/version"
	"github.com/pgplex/pgschema/ir"
)

// DumpFormatter handles formatting SQL output for database dumps
type DumpFormatter struct {
	dbVersion    string
	targetSchema string
	noComments   bool
}

// NewDumpFormatter creates a new DumpFormatter
func NewDumpFormatter(dbVersion string, targetSchema string, noComments bool) *DumpFormatter {
	return &DumpFormatter{
		dbVersion:    dbVersion,
		targetSchema: targetSchema,
		noComments:   noComments,
	}
}

// FormatSingleFile formats SQL output for single-file dump with pg_dump-style headers
func (f *DumpFormatter) FormatSingleFile(diffs []diff.Diff) string {
	var output strings.Builder

	// Generate and write header
	header := f.generateDumpHeader()
	output.WriteString(header)

	// Format SQL with pg_dump-style formatting
	for i, step := range diffs {
		if step.Type == diff.DiffTypeComment || strings.HasSuffix(step.Type.String(), ".comment") {
			// For comments, just write the raw SQL without DDL header
			if i > 0 {
				output.WriteString("\n") // Add separator from previous statement
			}
			for _, stmt := range step.Statements {
				output.WriteString(stmt.SQL)
				output.WriteString("\n")
			}
		} else {
			// Add object comment header (unless --no-comments is set)
			if !f.noComments {
				output.WriteString(f.formatObjectCommentHeader(step))
			} else if i > 0 {
				output.WriteString("\n") // Add separator between statements
			}

			// Add the SQL statements
			for _, stmt := range step.Statements {
				output.WriteString(stmt.SQL)
				output.WriteString("\n")
			}
		}

		// Add newline after SQL, and extra newline only if not last item
		if i < len(diffs)-1 {
			output.WriteString("\n")
		}
	}

	// Add trailing newline (Unix convention)
	output.WriteString("\n")
	return output.String()
}

// FormatMultiFile creates multiple SQL files organized by object type
func (f *DumpFormatter) FormatMultiFile(diffs []diff.Diff, outputPath string) error {
	// Create base directory
	baseDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Organization by object type
	filesByType := make(map[string]map[string][]diff.Diff)
	// Track insertion order per directory to preserve dependency ordering from the diff package.
	// The diff package topologically sorts views, functions, tables, and types, so preserving
	// the order in which each object first appears maintains correct dependency ordering.
	orderByDir := make(map[string][]string)
	includes := []string{}

	// Group diffs by object type and name, tracking first-appearance order
	for _, step := range diffs {
		objType := step.Type.String()

		// Determine directory and object name
		var dir string
		if step.Type == diff.DiffTypeComment {
			// Special handling for comments - use parent directory
			dir = f.getCommentParentDirectory(step)
		} else {
			dir = f.getObjectDirectory(objType)
		}
		objName := f.getGroupingName(step)

		if filesByType[dir] == nil {
			filesByType[dir] = make(map[string][]diff.Diff)
		}

		// Track first appearance of each object name per directory
		if _, exists := filesByType[dir][objName]; !exists {
			orderByDir[dir] = append(orderByDir[dir], objName)
		}

		filesByType[dir][objName] = append(filesByType[dir][objName], step)
	}

	// Create files in dependency order
	orderedDirs := []string{"types", "domains", "sequences", "functions", "procedures", "tables", "views", "materialized_views", "default_privileges", "privileges"}

	for _, dir := range orderedDirs {
		if objects, exists := filesByType[dir]; exists {
			// Create directory
			dirPath := filepath.Join(baseDir, dir)
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
			}

			// Use the order objects first appeared in the diffs.
			// This preserves dependency ordering from the diff package (e.g., topological
			// sort for views, tables, functions) instead of sorting alphabetically.
			objNames := orderByDir[dir]

			// Create files for each object
			for _, objName := range objNames {
				objSteps := objects[objName]
				fileName := f.sanitizeFileName(objName) + ".sql"
				filePath := filepath.Join(dirPath, fileName)
				relativePath := filepath.Join(dir, fileName)

				// Write object file
				if err := f.writeObjectFile(filePath, objSteps); err != nil {
					return fmt.Errorf("failed to write file %s: %w", filePath, err)
				}

				// Add include statement
				includes = append(includes, fmt.Sprintf("\\i %s", relativePath))
			}
		}
	}

	// Create main file with header and includes
	mainFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create main file: %w", err)
	}
	defer mainFile.Close()

	// Write header
	header := f.generateDumpHeader()
	mainFile.WriteString(header)

	// Write includes
	for _, include := range includes {
		mainFile.WriteString(include + "\n")
	}

	return nil
}

// generateDumpHeader generates the header for database dumps with metadata
func (f *DumpFormatter) generateDumpHeader() string {
	var header strings.Builder

	header.WriteString("--\n")
	header.WriteString("-- pgschema database dump\n")
	header.WriteString("--\n")
	header.WriteString("\n")

	header.WriteString(fmt.Sprintf("-- Dumped from database version %s\n", f.dbVersion))
	header.WriteString(fmt.Sprintf("-- Dumped by pgschema version %s\n", version.App()))
	header.WriteString("\n")
	header.WriteString("\n")
	return header.String()
}

// writeObjectFile writes a single object file with its SQL statements
func (f *DumpFormatter) writeObjectFile(filePath string, diffs []diff.Diff) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	for i, step := range diffs {
		isComment := step.Type == diff.DiffTypeComment || strings.HasSuffix(step.Type.String(), ".comment")

		if isComment {
			// For comments, add a blank line before the comment
			file.WriteString("\n")
			for _, stmt := range step.Statements {
				file.WriteString(stmt.SQL)
				file.WriteString("\n")
			}
		} else {
			// For non-comment statements, check if we need spacing
			if i > 0 {
				// Check if previous step was a comment
				prevIsComment := diffs[i-1].Type == diff.DiffTypeComment || strings.HasSuffix(diffs[i-1].Type.String(), ".comment")
				if prevIsComment {
					// Add extra newline after comment before next object
					file.WriteString("\n")
				} else {
					// Normal spacing between non-comment objects - just one newline
					file.WriteString("\n")
				}
			}

			// Add object comment header (unless --no-comments is set)
			if !f.noComments {
				file.WriteString(f.formatObjectCommentHeader(step))
			}

			// Print the SQL statements
			for _, stmt := range step.Statements {
				file.WriteString(stmt.SQL)
				file.WriteString("\n")
			}
		}
	}

	return nil
}

// getObjectDirectory returns the directory name for an object type
func (f *DumpFormatter) getObjectDirectory(objectType string) string {
	switch objectType {
	case "type":
		return "types"
	case "domain":
		return "domains"
	case "sequence":
		return "sequences"
	case "function":
		return "functions"
	case "procedure":
		return "procedures"
	case "table":
		return "tables"
	case "schema":
		return "schemas"
	case "view":
		return "views"
	case "materialized_view":
		return "materialized_views"
	case "table.index", "table.trigger", "table.constraint", "table.policy", "table.rls", "table.comment", "table.column.comment", "table.index.comment":
		// These are included with their tables
		return "tables"
	case "view.trigger":
		// View triggers are included with their views
		return "views"
	case "view.comment":
		// View comments are included with their views
		return "views"
	case "materialized_view.comment", "materialized_view.index", "materialized_view.index.comment":
		// Materialized view comments/indexes are included with their materialized views
		return "materialized_views"
	case "comment":
		// Comments handled separately in FormatMultiFile
		return "tables" // fallback, will be overridden
	case "default_privilege":
		return "default_privileges"
	case "privilege", "revoked_default_privilege", "column_privilege":
		return "privileges"
	default:
		return "misc"
	}
}

// getGroupingName determines the appropriate name for grouping objects into files
func (f *DumpFormatter) getGroupingName(step diff.Diff) string {
	// For table-related objects, try to extract the table name from Source
	switch step.Type {
	case diff.DiffTypeSchema:
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.Schema:
				return obj.Name
			}
		}
		return step.Path
	case diff.DiffTypeTableIndex, diff.DiffTypeTableTrigger, diff.DiffTypeTableConstraint, diff.DiffTypeTablePolicy, diff.DiffTypeTableRLS, diff.DiffTypeTableComment, diff.DiffTypeTableColumnComment, diff.DiffTypeTableIndexComment:
		if tableName := f.extractTableNameFromContext(step); tableName != "" {
			return tableName
		}
		// Fallback: extract table name from path
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return table name
		}
	case diff.DiffTypeViewTrigger:
		// For view triggers, group with view
		if tableName := f.extractTableNameFromContext(step); tableName != "" {
			return tableName
		}
		// Fallback: extract view name from path
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return view name
		}
	case diff.DiffTypeViewComment:
		// For view comments, group with view
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.View:
				return obj.Name // View comments group with view
			}
		}
		// Fallback: extract view name from path
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return view name
		}
	case diff.DiffTypeMaterializedViewComment:
		// For materialized view comments, group with materialized view
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.View:
				return obj.Name // Materialized view comments group with materialized view
			}
		}
		// Fallback: extract materialized view name from path
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return materialized view name
		}
	case diff.DiffTypeMaterializedViewIndex, diff.DiffTypeMaterializedViewIndexComment:
		// For materialized view indexes and their comments, group with materialized view
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.Index:
				return obj.Table // Index's Table field contains the materialized view name
			}
		}
		// Fallback: extract materialized view name from path (schema.mv.index)
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return materialized view name
		}
	case diff.DiffTypeComment:
		// For legacy comments, we need to determine the parent object
		// For index comments, group with parent table
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.Index:
				return obj.Table // Group index comments with their table
			case *ir.Table:
				return obj.Name // Table and column comments group with table
			case *ir.View:
				return obj.Name // View comments group with view
			}
		}
		// Fallback: extract parent object name from object path
		// Path format: "schema.table" or "schema.table.column" for table/column comments
		// Path format: "schema.view" for view comments
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return parent object name (table/view)
		}
	case diff.DiffTypeDefaultPrivilege:
		// For default privileges, group by object type
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.DefaultPrivilege:
				return string(obj.ObjectType) // Group by TABLES, SEQUENCES, etc.
			}
		}
		// Fallback: extract from path (default_privileges.TABLES.grantee)
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return object type
		}
	case diff.DiffTypePrivilege:
		// For explicit privileges, group by object type
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.Privilege:
				return string(obj.ObjectType) // Group by TABLE, FUNCTION, etc.
			}
		}
		// Fallback: extract from path (privileges.TABLE.name.grantee)
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return object type
		}
	case diff.DiffTypeRevokedDefaultPrivilege:
		// For revoked default privileges, group by object type
		if step.Source != nil {
			switch obj := step.Source.(type) {
			case *ir.RevokedDefaultPrivilege:
				return string(obj.ObjectType) // Group by FUNCTION, TYPE, etc.
			}
		}
		// Fallback: extract from path (revoked_default.FUNCTION.name)
		if parts := strings.Split(step.Path, "."); len(parts) >= 2 {
			return parts[1] // Return object type
		}
	case diff.DiffTypeColumnPrivilege:
		// For column privileges, group by TABLE (always table-based)
		return "TABLE"
	}

	// For standalone objects or if table name extraction fails, use object name
	return f.getObjectName(step.Path)
}

// extractTableNameFromContext extracts table name from the Source in diffContext
func (f *DumpFormatter) extractTableNameFromContext(step diff.Diff) string {
	if step.Source == nil {
		return ""
	}

	// Try to extract table name based on the type of Source
	switch obj := step.Source.(type) {
	case *ir.Index:
		return obj.Table
	case *ir.RLSPolicy:
		return obj.Table
	case *ir.Trigger:
		return obj.Table
	case *ir.Constraint:
		return obj.Table
	// For other table-related objects, we might need to parse or handle differently
	default:
		return ""
	}
}

// getCommentParentDirectory determines the directory for comment statements based on their parent object
func (f *DumpFormatter) getCommentParentDirectory(step diff.Diff) string {
	if step.Source != nil {
		switch obj := step.Source.(type) {
		case *ir.View:
			// Check if it's a materialized view
			if obj.Materialized {
				return "materialized_views"
			}
			return "views"
		case *ir.Table, *ir.Index:
			return "tables"
		}
	}
	// Fallback to tables for unknown comment types
	return "tables"
}

// getObjectName extracts the object name from the object path
func (f *DumpFormatter) getObjectName(objectPath string) string {
	if strings.Contains(objectPath, ".") {
		parts := strings.Split(objectPath, ".")
		// Return the last component
		return parts[len(parts)-1]
	}
	return objectPath
}

// sanitizeFileName converts an object name to a valid filename
func (f *DumpFormatter) sanitizeFileName(name string) string {
	// Replace non-alphanumeric characters with underscores
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	sanitized := reg.ReplaceAllString(name, "_")

	// Remove leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	// Convert to lowercase for consistency
	return strings.ToLower(sanitized)
}

// formatObjectCommentHeader generates the comment header for an object
func (f *DumpFormatter) formatObjectCommentHeader(step diff.Diff) string {
	var output strings.Builder

	// Add DDL separator with comment header for non-comment statements
	output.WriteString("--\n")

	// Determine schema name for comment
	commentSchemaName := f.getCommentSchemaName(step.Path)

	// Get object name from source object to preserve names with dots
	var objectName string
	// Special handling for functions and procedures to include signature
	switch obj := step.Source.(type) {
	case *ir.Function:
		objectName = obj.Name + "(" + obj.GetArguments() + ")"
	case *ir.Procedure:
		objectName = obj.Name + "(" + obj.GetArguments() + ")"
	default:
		// Use the GetObjectName interface method for all other types
		objectName = step.Source.GetObjectName()
	}

	parts := strings.Split(step.Type.String(), ".")
	objectType := parts[len(parts)-1]

	// Always use the actual object type for consistency between single-file and multi-file modes
	displayType := strings.ToUpper(objectType)

	// Special handling for materialized views
	if displayType == "MATERIALIZED_VIEW" {
		// Convert underscore to space for proper SQL comment format
		displayType = "MATERIALIZED VIEW"
	} else if displayType == "VIEW" && step.Source != nil {
		// Also check if a regular view is actually materialized
		if view, ok := step.Source.(*ir.View); ok && view.Materialized {
			displayType = "MATERIALIZED VIEW"
		}
	}

	output.WriteString(fmt.Sprintf("-- Name: %s; Type: %s; Schema: %s; Owner: -\n", objectName, displayType, commentSchemaName))
	output.WriteString("--\n")
	output.WriteString("\n")

	return output.String()
}

// getCommentSchemaName determines the schema name for comment headers
func (f *DumpFormatter) getCommentSchemaName(path string) string {
	if strings.Contains(path, ".") {
		parts := strings.Split(path, ".")
		if len(parts) >= 2 && parts[0] == f.targetSchema {
			return "-"
		} else if len(parts) >= 2 {
			return parts[0]
		}
	}
	return path
}
