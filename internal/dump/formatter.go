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
	dbVersion     string
	targetSchema  string
	noComments    bool
	qualifySchema bool
	// dataTables are the reference tables whose \copy directives are appended
	// after every object (see FormatDataDirectives).
	dataTables []*ir.Table
}

// SetDataTables sets the reference tables whose \copy directives follow all
// objects, in single-file output and in the multi-file main file.
func (f *DumpFormatter) SetDataTables(tables []*ir.Table) {
	f.dataTables = tables
}

// NewDumpFormatter creates a new DumpFormatter. When qualifySchema is true, comment
// headers keep the target schema name (instead of "-") to match the forced
// schema-qualified DDL emitted by `dump --qualify-schema`.
func NewDumpFormatter(dbVersion string, targetSchema string, noComments bool, qualifySchema bool) *DumpFormatter {
	return &DumpFormatter{
		dbVersion:     dbVersion,
		targetSchema:  targetSchema,
		noComments:    noComments,
		qualifySchema: qualifySchema,
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

	output.WriteString(f.FormatDataDirectives(f.dataTables))

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

	// Group diffs into one file per object. Each file is keyed by its directory
	// and object name; the file order records the first appearance of each file
	// in the diff sequence.
	type fileKey struct{ dir, name string }
	stepsByFile := make(map[fileKey][]diff.Diff)
	var fileOrder []fileKey
	var tables []*ir.Table
	var views []*ir.View

	for _, step := range diffs {
		var dir string
		if step.Type == diff.DiffTypeComment {
			// Special handling for comments - use parent directory
			dir = f.getCommentParentDirectory(step)
		} else {
			dir = f.getObjectDirectory(step.Type.String())
		}
		key := fileKey{dir: dir, name: f.getGroupingName(step)}

		if _, exists := stepsByFile[key]; !exists {
			fileOrder = append(fileOrder, key)
		}
		stepsByFile[key] = append(stepsByFile[key], step)

		switch src := step.Source.(type) {
		case *ir.Table:
			tables = append(tables, src)
		case *ir.View:
			views = append(views, src)
		}
	}

	// Compute the include order. The diff package already emits statements in
	// dependency order, so the first appearance of each file is the baseline.
	// One adjustment is needed because a table's triggers, policies, and
	// deferred constraints are bundled into the table's file even though the
	// diff emits them after functions: any function whose signature does not
	// reference a table or view is hoisted ahead of the first table file so
	// the bundled statements can resolve it. Functions that do use a relation's
	// row type stay where the diff placed them, after that relation (#580).
	firstTable := len(fileOrder)
	for i, key := range fileOrder {
		if key.dir == "tables" {
			firstTable = i
			break
		}
	}
	relations := diff.RelationLookup(tables, views)
	orderedFiles := make([]fileKey, 0, len(fileOrder))
	orderedFiles = append(orderedFiles, fileOrder[:firstTable]...)
	var rest []fileKey
	for _, key := range fileOrder[firstTable:] {
		if key.dir == "functions" && !f.functionFileReferencesRelation(stepsByFile[key], relations) {
			orderedFiles = append(orderedFiles, key)
		} else {
			rest = append(rest, key)
		}
	}
	orderedFiles = append(orderedFiles, rest...)

	includes := make([]string, 0, len(orderedFiles))
	for _, key := range orderedFiles {
		dirPath := filepath.Join(baseDir, key.dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dirPath, err)
		}

		fileName := f.sanitizeFileName(key.name) + ".sql"
		filePath := filepath.Join(dirPath, fileName)
		if err := f.writeObjectFile(filePath, stepsByFile[key]); err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}

		includes = append(includes, fmt.Sprintf("\\i %s", filepath.Join(key.dir, fileName)))
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

	mainFile.WriteString(f.FormatDataDirectives(f.dataTables))

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

// functionFileReferencesRelation reports whether any function in a functions/
// file depends on a table or view existing at creation time. Such a file must
// keep the diff's placement after that relation rather than being hoisted
// ahead of the tables.
//
// For SQL-language functions this is the diff's full check, signature plus
// body, because PostgreSQL resolves a SQL body against the catalog when the
// function is created. For other languages only the signature counts: a
// plpgsql body is not validated against relations until it runs, and treating
// its table mentions as dependencies would demote trigger functions that write
// to other tables below the table file that bundles their trigger.
func (f *DumpFormatter) functionFileReferencesRelation(steps []diff.Diff, relations map[string]struct{}) bool {
	for _, step := range steps {
		fn, ok := step.Source.(*ir.Function)
		if !ok {
			continue
		}
		if strings.EqualFold(fn.Language, "sql") {
			if diff.FunctionReferencesRelation(fn, relations) {
				return true
			}
		} else if diff.FunctionSignatureReferencesRelation(fn, relations) {
			return true
		}
	}
	return false
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
	case "aggregate":
		return "aggregates"
	case "table":
		return "tables"
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
	case *ir.Aggregate:
		argsClause := obj.Arguments
		if argsClause == "" {
			argsClause = "*"
		}
		objectName = obj.Name + "(" + argsClause + ")"
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

// commentHeader renders the pg_dump-style header used before every object.
func commentHeader(objectName, displayType, schemaName string) string {
	return fmt.Sprintf("--\n-- Name: %s; Type: %s; Schema: %s; Owner: -\n--\n\n", objectName, displayType, schemaName)
}

// getCommentSchemaName determines the schema name for comment headers
func (f *DumpFormatter) getCommentSchemaName(path string) string {
	if strings.Contains(path, ".") {
		parts := strings.Split(path, ".")
		if len(parts) >= 2 && parts[0] == f.targetSchema {
			// With forced qualification, keep the schema name rather than collapsing
			// the target schema to "-", so headers match the qualified DDL.
			if f.qualifySchema {
				return parts[0]
			}
			return "-"
		} else if len(parts) >= 2 {
			return parts[0]
		}
	}
	return path
}
