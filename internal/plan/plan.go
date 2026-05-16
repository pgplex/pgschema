package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/pgplex/pgschema/internal/color"
	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/fingerprint"
	"github.com/pgplex/pgschema/internal/version"
)

// DirectiveType represents the different types of directives
type DirectiveType string

const (
	DirectiveTypeWait DirectiveType = "wait"
)

// String returns the string representation of DirectiveType
func (dt DirectiveType) String() string {
	return string(dt)
}

// Directive represents a special directive for execution (wait, assert, etc.)
type Directive struct {
	Type    DirectiveType `json:"type"`    // DirectiveTypeWait, etc.
	Message string        `json:"message"` // Auto-generated descriptive message
}

// Step represents a single execution step with SQL and optional directive
type Step struct {
	SQL       string     `json:"sql"`
	Directive *Directive `json:"directive,omitempty"`
	// Metadata for summary generation
	Type      string `json:"type,omitempty"`      // e.g., "table", "index"
	Operation string `json:"operation,omitempty"` // e.g., "create", "alter", "drop"
	Path      string `json:"path,omitempty"`      // e.g., "public.users"
}

// ExecutionGroup represents a group of steps that should be executed together
type ExecutionGroup struct {
	Steps []Step `json:"steps"`
}

// Plan represents the migration plan between two DDL states
type Plan struct {
	// Version information
	Version         string `json:"version"`
	PgschemaVersion string `json:"pgschema_version"`

	// When the plan was created
	CreatedAt time.Time `json:"created_at"`

	// Source database fingerprint when plan was created
	SourceFingerprint *fingerprint.SchemaFingerprint `json:"source_fingerprint,omitempty"`

	// Groups is the ordered list of execution groups
	Groups []ExecutionGroup `json:"groups"`

	// SourceDiffs stores original diff information for summary calculation
	// This field is only serialized in debug mode
	SourceDiffs []diff.Diff `json:"source_diffs,omitempty"`
}

// PlanSummary provides counts of changes by type
type PlanSummary struct {
	Total   int                    `json:"total"`
	Add     int                    `json:"add"`
	Change  int                    `json:"change"`
	Destroy int                    `json:"destroy"`
	ByType  map[string]TypeSummary `json:"by_type"`
}

// TypeSummary provides counts for a specific object type
type TypeSummary struct {
	Add     int `json:"add"`
	Change  int `json:"change"`
	Destroy int `json:"destroy"`
}

// Type represents the database object types in dependency order
type Type string

const (
	TypeExtension               Type = "extensions"
	TypeSchema                  Type = "schemas"
	TypeType                    Type = "types"
	TypeFunction                Type = "functions"
	TypeProcedure               Type = "procedures"
	TypeSequence                Type = "sequences"
	TypeTable                   Type = "tables"
	TypeView                    Type = "views"
	TypeMaterializedView        Type = "materialized views"
	TypeIndex                   Type = "indexes"
	TypeTrigger                 Type = "triggers"
	TypePolicy                  Type = "policies"
	TypeColumn                  Type = "columns"
	TypeRLS                     Type = "rls"
	TypeDefaultPrivilege        Type = "default privileges"
	TypePrivilege               Type = "privileges"
	TypeColumnPrivilege         Type = "column privileges"
	TypeRevokedDefaultPrivilege Type = "revoked default privileges"
)

// SQLFormat represents the different output formats for SQL generation
type SQLFormat string

const (
	// SQLFormatRaw outputs just the raw SQL statements without additional formatting
	SQLFormatRaw SQLFormat = "raw"
	// Human-readable format with comments
	SQLFormatHuman SQLFormat = "human"
)

// getObjectOrder returns the dependency order for database objects
func getObjectOrder() []Type {
	return []Type{
		TypeExtension,
		TypeSchema,
		TypeDefaultPrivilege,
		TypeType,
		TypeFunction,
		TypeProcedure,
		TypeSequence,
		TypeTable,
		TypeView,
		TypeMaterializedView,
		TypeIndex,
		TypeTrigger,
		TypePolicy,
		TypeColumn,
		TypeRLS,
		TypePrivilege,
		TypeColumnPrivilege,
		TypeRevokedDefaultPrivilege,
	}
}

// ========== PUBLIC METHODS ==========

// groupDiffs groups diffs into execution groups with configurable online operations
func groupDiffs(diffs []diff.Diff) []ExecutionGroup {
	if len(diffs) == 0 {
		return nil
	}

	var groups []ExecutionGroup
	var transactionalSteps []Step

	// Track newly created tables/materialized views to avoid concurrent rewrites for their indexes.
	// Single-pass: diffs are topologically sorted, so creates come before dependent index operations.
	// We build these maps incrementally as we process each diff.
	newlyCreatedTables := make(map[string]bool)
	newlyCreatedMaterializedViews := make(map[string]bool)

	// Convert diffs to steps
	for _, d := range diffs {
		// Track creates as we encounter them (before processing dependent operations)
		if d.Type == diff.DiffTypeTable && d.Operation == diff.DiffOperationCreate {
			newlyCreatedTables[d.Path] = true
		}
		if d.Type == diff.DiffTypeMaterializedView && d.Operation == diff.DiffOperationCreate {
			newlyCreatedMaterializedViews[d.Path] = true
		}
		// Try to generate rewrites if online operations are enabled
		rewriteSteps := generateRewrite(d, newlyCreatedTables, newlyCreatedMaterializedViews)

		if len(rewriteSteps) > 0 {
			// For operations with rewrites, create one step per rewrite statement
			for _, rewriteStep := range rewriteSteps {
				step := Step{
					SQL:       rewriteStep.SQL,
					Type:      d.Type.String(),
					Operation: d.Operation.String(),
					Path:      d.Path,
					Directive: rewriteStep.Directive,
				}

				// Check if this step needs isolation (has directive or cannot run in transaction)
				needsIsolation := step.Directive != nil || !rewriteStep.CanRunInTransaction

				if needsIsolation {
					// Flush any pending transactional steps
					if len(transactionalSteps) > 0 {
						groups = append(groups, ExecutionGroup{Steps: transactionalSteps})
						transactionalSteps = nil
					}

					// Add this step in its own group
					groups = append(groups, ExecutionGroup{Steps: []Step{step}})
				} else {
					// Accumulate transactional steps
					transactionalSteps = append(transactionalSteps, step)
				}
			}
		} else {
			// For operations without rewrites, create one step per canonical statement
			for _, stmt := range d.Statements {
				step := Step{
					SQL:       stmt.SQL,
					Type:      d.Type.String(),
					Operation: d.Operation.String(),
					Path:      d.Path,
				}
				// Canonical statements don't have directives
				transactionalSteps = append(transactionalSteps, step)
			}
		}
	}

	// Flush remaining transactional steps
	if len(transactionalSteps) > 0 {
		groups = append(groups, ExecutionGroup{Steps: transactionalSteps})
	}

	return groups
}

// NewPlan creates a new plan from a list of diffs with online operations enabled
func NewPlan(diffs []diff.Diff) *Plan {
	// Use environment variable for timestamp if provided, otherwise use current time
	createdAt := time.Now().Truncate(time.Second)
	if testTime := os.Getenv("PGSCHEMA_TEST_TIME"); testTime != "" {
		if parsedTime, err := time.Parse(time.RFC3339, testTime); err == nil {
			createdAt = parsedTime
		}
	}

	plan := &Plan{
		Version:         version.PlanFormat(),
		PgschemaVersion: version.App(),
		CreatedAt:       createdAt,
		Groups:          groupDiffs(diffs),
		SourceDiffs:     diffs,
	}

	return plan
}

// NewPlanWithFingerprint creates a new plan from diffs and includes source fingerprint
func NewPlanWithFingerprint(diffs []diff.Diff, sourceFingerprint *fingerprint.SchemaFingerprint) *Plan {
	plan := NewPlan(diffs)
	plan.SourceFingerprint = sourceFingerprint
	return plan
}

// HasAnyChanges checks if the plan contains any changes by examining the groups
func (p *Plan) HasAnyChanges() bool {
	for _, g := range p.Groups {
		if len(g.Steps) > 0 {
			return true
		}
	}
	return false
}

// HumanColored returns a human-readable summary of the plan with color support
func (p *Plan) HumanColored(enableColor bool) string {
	c := color.New(enableColor)
	var summary strings.Builder

	// Calculate summary from diffs
	summaryData := p.calculateSummaryFromSteps()

	if summaryData.Total == 0 {
		summary.WriteString("No changes detected.\n")
		return summary.String()
	}

	// Write header with overall summary (colored like Terraform)
	summary.WriteString(c.FormatPlanHeader(summaryData.Add, summaryData.Change, summaryData.Destroy) + "\n\n")

	// Write summary by type with colors
	summary.WriteString(c.Bold("Summary by type:") + "\n")
	for _, objType := range getObjectOrder() {
		objTypeStr := string(objType)
		if typeSummary, exists := summaryData.ByType[objTypeStr]; exists && (typeSummary.Add > 0 || typeSummary.Change > 0 || typeSummary.Destroy > 0) {
			line := c.FormatSummaryLine(objTypeStr, typeSummary.Add, typeSummary.Change, typeSummary.Destroy)
			summary.WriteString(line + "\n")
		}
	}
	summary.WriteString("\n")

	// Detailed changes by type with symbols
	for _, objType := range getObjectOrder() {
		objTypeStr := string(objType)
		if typeSummary, exists := summaryData.ByType[objTypeStr]; exists && (typeSummary.Add > 0 || typeSummary.Change > 0 || typeSummary.Destroy > 0) {
			// Capitalize first letter for display
			displayName := strings.ToUpper(objTypeStr[:1]) + objTypeStr[1:]
			p.writeDetailedChangesFromSteps(&summary, displayName, objTypeStr, c)
		}
	}

	// Add DDL section if there are changes
	if summaryData.Total > 0 {
		summary.WriteString(c.Bold("DDL to be executed:") + "\n")
		summary.WriteString(strings.Repeat("-", 50) + "\n\n")
		migrationSQL := p.ToSQL(SQLFormatHuman)
		if migrationSQL != "" {
			summary.WriteString(migrationSQL)
			if !strings.HasSuffix(migrationSQL, "\n") {
				summary.WriteString("\n")
			}
		} else {
			summary.WriteString("-- No DDL statements generated\n")
		}
	}

	return summary.String()
}

// ToSQL returns the SQL statements with formatting based on the specified format
func (p *Plan) ToSQL(format SQLFormat) string {
	// Build SQL output from groups
	var sqlOutput strings.Builder

	for groupIdx, group := range p.Groups {
		// Add transaction group comment for human-readable format
		if format == SQLFormatHuman && len(p.Groups) > 1 {
			sqlOutput.WriteString(fmt.Sprintf("-- Transaction Group #%d\n", groupIdx+1))
		}

		for stepIdx, step := range group.Steps {
			if step.Directive != nil {
				// Handle directive statements
				sqlOutput.WriteString(fmt.Sprintf("-- pgschema:%s\n", step.Directive.Type.String()))
				sqlOutput.WriteString(step.SQL)
				sqlOutput.WriteString("\n")
			} else {
				// Handle regular SQL statements
				sqlOutput.WriteString(step.SQL)
				sqlOutput.WriteString("\n")
			}

			// Add blank line between steps except for the last one in the last group
			if stepIdx < len(group.Steps)-1 || groupIdx < len(p.Groups)-1 {
				sqlOutput.WriteString("\n")
			}
		}
	}

	return sqlOutput.String()
}

// ToJSON returns the plan as structured JSON with only changed statements
func (p *Plan) ToJSON() (string, error) {
	return p.ToJSONWithDebug(false)
}

// ToJSONWithDebug returns the plan as structured JSON with optional source field inclusion
func (p *Plan) ToJSONWithDebug(includeSource bool) (string, error) {
	var buf strings.Builder
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	// Create a copy of the plan to control SourceDiffs serialization
	planCopy := *p
	if !includeSource {
		// Clear SourceDiffs in normal mode to keep JSON clean
		planCopy.SourceDiffs = nil
	}

	if err := encoder.Encode(&planCopy); err != nil {
		return "", fmt.Errorf("failed to marshal plan to JSON: %w", err)
	}

	// Remove the trailing newline that encoder.Encode adds
	result := buf.String()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// FromJSON creates a Plan from JSON data
func FromJSON(jsonData []byte) (*Plan, error) {
	var plan Plan
	if err := json.Unmarshal(jsonData, &plan); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan JSON: %w", err)
	}
	return &plan, nil
}

// ========== PRIVATE METHODS ==========

// calculateSummaryFromSteps calculates summary statistics from the plan diffs
func (p *Plan) calculateSummaryFromSteps() PlanSummary {
	summary := PlanSummary{
		ByType: make(map[string]TypeSummary),
	}

	// For tables, we need to group by table path to avoid counting duplicates
	// For other object types, count each operation individually

	// Track table operations by table path
	tableOperations := make(map[string]string) // table_path -> operation

	// Track tables that have sub-resource changes (these should be counted as modified)
	tablesWithSubResources := make(map[string]bool) // table_path -> true

	// Track view operations by view path (regular views only)
	viewOperations := make(map[string]string) // view_path -> operation

	// Track views that have sub-resource changes (these should be counted as modified)
	viewsWithSubResources := make(map[string]bool) // view_path -> true

	// Track materialized view operations by path
	materializedViewOperations := make(map[string]string) // materialized_view_path -> operation

	// Track materialized views that have sub-resource changes
	materializedViewsWithSubResources := make(map[string]bool) // materialized_view_path -> true

	// Track materialized views that have "recreate" operations (DROP that will be followed by CREATE)
	// These should be counted as modifications, not adds
	materializedViewsRecreating := make(map[string]bool) // materialized_view_path -> true

	// Track non-table/non-view/non-materialized-view operations
	nonTableOperations := make(map[string][]string) // objType -> []operations

	// Use source diffs for summary calculation if available,
	// otherwise use steps metadata (for plans loaded from JSON)
	var dataToProcess []struct {
		Type      string
		Operation string
		Path      string
	}

	if len(p.SourceDiffs) > 0 {
		// Use SourceDiffs (for freshly generated plans)
		for _, diff := range p.SourceDiffs {
			dataToProcess = append(dataToProcess, struct {
				Type      string
				Operation string
				Path      string
			}{
				Type:      diff.Type.String(),
				Operation: diff.Operation.String(),
				Path:      diff.Path,
			})
		}
	} else {
		// Use Steps metadata (for plans loaded from JSON)
		for _, group := range p.Groups {
			for _, step := range group.Steps {
				if step.Type != "" && step.Operation != "" && step.Path != "" {
					dataToProcess = append(dataToProcess, struct {
						Type      string
						Operation string
						Path      string
					}{
						Type:      step.Type,
						Operation: step.Operation,
						Path:      step.Path,
					})
				}
			}
		}
	}

	// Single-pass: process all steps, determining parent type from step.Type prefix
	// Sub-resource types encode their parent: "table.index", "view.index", "materialized_view.index"
	for _, step := range dataToProcess {
		// Normalize object type to match the expected format (add 's' for plural)
		stepObjTypeStr := step.Type
		if !strings.HasSuffix(stepObjTypeStr, "s") {
			stepObjTypeStr += "s"
		}

		if stepObjTypeStr == "tables" {
			// For tables, track unique table paths and their primary operation
			tableOperations[step.Path] = step.Operation
		} else if stepObjTypeStr == "views" {
			// For views, track unique view paths and their primary operation
			viewOperations[step.Path] = step.Operation
		} else if stepObjTypeStr == "materialized_views" {
			// For materialized views, track unique paths and their primary operation
			// If this is a "recreate" operation, mark it so subsequent "create" is treated as modify
			if step.Operation == "recreate" {
				materializedViewsRecreating[step.Path] = true
			}
			materializedViewOperations[step.Path] = step.Operation
		} else if isSubResource(step.Type) {
			// For sub-resources, determine parent type from step.Type prefix
			// Types are: "table.index", "table.column", "view.comment", "materialized_view.index", etc.
			parentPath := extractTablePathFromSubResource(step.Path, step.Type)
			if parentPath != "" {
				if strings.HasPrefix(step.Type, "materialized_view.") {
					// Parent is a materialized view
					materializedViewsWithSubResources[parentPath] = true
				} else if strings.HasPrefix(step.Type, "view.") {
					// Parent is a view
					viewsWithSubResources[parentPath] = true
				} else {
					// Parent is a table (table.index, table.column, table.constraint, etc.)
					tablesWithSubResources[parentPath] = true
				}
			}
		} else {
			// For non-table/non-view objects, track each operation
			nonTableOperations[stepObjTypeStr] = append(nonTableOperations[stepObjTypeStr], step.Operation)
		}
	}

	// Count table operations (one per unique table)
	// Include both direct table operations and tables with sub-resource changes
	allAffectedTables := make(map[string]string)

	// First, add direct table operations
	for tablePath, operation := range tableOperations {
		allAffectedTables[tablePath] = operation
	}

	// Then, add tables that only have sub-resource changes (count as "alter")
	for tablePath := range tablesWithSubResources {
		if _, alreadyCounted := allAffectedTables[tablePath]; !alreadyCounted {
			allAffectedTables[tablePath] = "alter" // Sub-resource changes = table modification
		}
	}

	if len(allAffectedTables) > 0 {
		stats := summary.ByType["tables"]
		for _, operation := range allAffectedTables {
			switch operation {
			case "create":
				stats.Add++
				summary.Add++
			case "alter":
				stats.Change++
				summary.Change++
			case "drop":
				stats.Destroy++
				summary.Destroy++
			}
		}
		summary.ByType["tables"] = stats
	}

	// Count view operations (one per unique view)
	// Include both direct view operations and views with sub-resource changes
	allAffectedViews := make(map[string]string)

	// First, add direct view operations
	for viewPath, operation := range viewOperations {
		allAffectedViews[viewPath] = operation
	}

	// Then, add views that only have sub-resource changes (count as "alter")
	for viewPath := range viewsWithSubResources {
		if _, alreadyCounted := allAffectedViews[viewPath]; !alreadyCounted {
			allAffectedViews[viewPath] = "alter" // Sub-resource changes = view modification
		}
	}

	if len(allAffectedViews) > 0 {
		stats := summary.ByType["views"]
		for _, operation := range allAffectedViews {
			switch operation {
			case "create":
				stats.Add++
				summary.Add++
			case "alter":
				stats.Change++
				summary.Change++
			case "drop":
				stats.Destroy++
				summary.Destroy++
			}
		}
		summary.ByType["views"] = stats
	}

	// Count materialized view operations (one per unique materialized view)
	// Include both direct materialized view operations and materialized views with sub-resource changes
	allAffectedMaterializedViews := make(map[string]string)

	// First, add direct materialized view operations
	for mvPath, operation := range materializedViewOperations {
		allAffectedMaterializedViews[mvPath] = operation
	}

	// Then, add materialized views that only have sub-resource changes (count as "alter")
	for mvPath := range materializedViewsWithSubResources {
		if _, alreadyCounted := allAffectedMaterializedViews[mvPath]; !alreadyCounted {
			allAffectedMaterializedViews[mvPath] = "alter" // Sub-resource changes = materialized view modification
		}
	}

	if len(allAffectedMaterializedViews) > 0 {
		stats := summary.ByType["materialized views"]
		for mvPath, operation := range allAffectedMaterializedViews {
			// If this path had a "recreate" operation, treat any subsequent "create" as a modify
			// because the object existed before and is being recreated due to dependencies
			if materializedViewsRecreating[mvPath] && operation == "create" {
				operation = "alter"
			}
			switch operation {
			case "create":
				stats.Add++
				summary.Add++
			case "alter", "recreate":
				// Both "alter" and "recreate" count as modifications
				stats.Change++
				summary.Change++
			case "drop":
				stats.Destroy++
				summary.Destroy++
			}
		}
		summary.ByType["materialized views"] = stats
	}

	// Count non-table/non-view/non-materialized-view operations (each operation counted individually)
	for objType, operations := range nonTableOperations {
		// Normalize object type to match the Type constants (replace underscores with spaces)
		normalizedObjType := strings.ReplaceAll(objType, "_", " ")
		stats := summary.ByType[normalizedObjType]
		for _, operation := range operations {
			switch operation {
			case "create":
				stats.Add++
				summary.Add++
			case "alter":
				stats.Change++
				summary.Change++
			case "drop":
				stats.Destroy++
				summary.Destroy++
			}
		}
		summary.ByType[normalizedObjType] = stats
	}

	summary.Total = summary.Add + summary.Change + summary.Destroy
	return summary
}

// writeDetailedChangesFromSteps writes detailed changes from plan diffs
func (p *Plan) writeDetailedChangesFromSteps(summary *strings.Builder, displayName, objType string, c *color.Color) {
	fmt.Fprintf(summary, "%s:\n", c.Bold(displayName))

	if objType == "tables" {
		// For tables, group all changes by table path to avoid duplicates
		p.writeTableChanges(summary, c)
	} else if objType == "views" {
		// For views, group all changes by view path to avoid duplicates
		p.writeViewChanges(summary, c)
	} else if objType == "materialized views" {
		// For materialized views, group all changes by path to avoid duplicates
		p.writeMaterializedViewChanges(summary, c)
	} else {
		// For non-table/non-view objects, use the original logic
		p.writeNonTableChanges(summary, objType, c)
	}

	summary.WriteString("\n")
}

// writeTableChanges handles table-specific output with proper grouping
func (p *Plan) writeTableChanges(summary *strings.Builder, c *color.Color) {
	// Group all changes by table path and track operations
	tableOperations := make(map[string]string) // table_path -> operation
	subResources := make(map[string][]struct {
		operation string
		path      string
		subType   string
		source    diff.DiffSource
	})

	// Track all seen operations globally to avoid duplicates across groups
	seenOperations := make(map[string]bool) // "path.operation.subType" -> true

	// Use source diffs for summary calculation
	for _, step := range p.SourceDiffs {
		// Normalize object type
		stepObjTypeStr := step.Type.String()
		if !strings.HasSuffix(stepObjTypeStr, "s") {
			stepObjTypeStr += "s"
		}

		if stepObjTypeStr == "tables" {
			// This is a table-level change, record the operation
			tableOperations[step.Path] = step.Operation.String()
		} else if isSubResource(step.Type.String()) && strings.HasPrefix(step.Type.String(), "table.") {
			// This is a table sub-resource change (skip view sub-resources)
			tablePath := extractTablePathFromSubResource(step.Path, step.Type.String())
			if tablePath != "" {
				// Deduplicate all operations based on (type, operation, path) triplet
				operationKey := step.Path + "." + step.Operation.String() + "." + step.Type.String()
				if !seenOperations[operationKey] {
					seenOperations[operationKey] = true
					subResources[tablePath] = append(subResources[tablePath], struct {
						operation string
						path      string
						subType   string
						source    diff.DiffSource
					}{
						operation: step.Operation.String(),
						path:      step.Path,
						subType:   step.Type.String(),
						source:    step.Source,
					})
				}
			}
		}
	}

	// Get all unique table paths (from both direct table changes and sub-resources)
	allTables := make(map[string]bool)
	for tablePath := range tableOperations {
		allTables[tablePath] = true
	}
	for tablePath := range subResources {
		allTables[tablePath] = true
	}

	// Sort table paths for consistent output
	var sortedTables []string
	for tablePath := range allTables {
		sortedTables = append(sortedTables, tablePath)
	}
	sort.Strings(sortedTables)

	// Display each table once with all its changes
	for _, tablePath := range sortedTables {
		var symbol string
		if operation, hasDirectChange := tableOperations[tablePath]; hasDirectChange {
			// Table has direct changes, use the operation to determine symbol
			switch operation {
			case "create":
				symbol = c.PlanSymbol("add")
			case "alter":
				symbol = c.PlanSymbol("change")
			case "drop":
				symbol = c.PlanSymbol("destroy")
			default:
				symbol = c.PlanSymbol("change")
			}
		} else {
			// Table has no direct changes, only sub-resource changes
			// Sub-resource changes to existing tables should always be considered modifications
			symbol = c.PlanSymbol("change")
		}

		fmt.Fprintf(summary, "  %s %s\n", symbol, getLastPathComponent(tablePath))

		// Show sub-resources for this table
		if subResourceList, exists := subResources[tablePath]; exists {
			// Sort sub-resources by type then path
			sort.Slice(subResourceList, func(i, j int) bool {
				if subResourceList[i].subType != subResourceList[j].subType {
					return subResourceList[i].subType < subResourceList[j].subType
				}
				return subResourceList[i].path < subResourceList[j].path
			})

			for _, subRes := range subResourceList {
				// Extract object name from source
				objectName := getObjectNameFromSource(subRes.source)

				// Handle online index replacement display
				if subRes.subType == diff.DiffTypeTableIndex.String() && subRes.operation == diff.DiffOperationAlter.String() {
					subSymbol := c.PlanSymbol("change")
					displaySubType := strings.TrimPrefix(subRes.subType, "table.")
					fmt.Fprintf(summary, "    %s %s (%s - concurrent rebuild)\n", subSymbol, objectName, displaySubType)
					continue
				}

				var subSymbol string
				switch subRes.operation {
				case "create":
					subSymbol = c.PlanSymbol("add")
				case "alter":
					subSymbol = c.PlanSymbol("change")
				case "drop":
					subSymbol = c.PlanSymbol("destroy")
				default:
					subSymbol = c.PlanSymbol("change")
				}
				// Clean up sub-resource type for display (remove "table." prefix)
				displaySubType := strings.TrimPrefix(subRes.subType, "table.")
				fmt.Fprintf(summary, "    %s %s (%s)\n", subSymbol, objectName, displaySubType)
			}
		}
	}
}

// writeViewChanges handles view-specific output with proper grouping
func (p *Plan) writeViewChanges(summary *strings.Builder, c *color.Color) {
	// Group all changes by view path and track operations
	viewOperations := make(map[string]string) // view_path -> operation
	subResources := make(map[string][]struct {
		operation string
		path      string
		subType   string
	})

	// Track all seen operations globally to avoid duplicates across groups
	seenOperations := make(map[string]bool) // "path.operation.subType" -> true

	// Use source diffs for summary calculation
	for _, step := range p.SourceDiffs {
		// Normalize object type
		stepObjTypeStr := step.Type.String()
		if !strings.HasSuffix(stepObjTypeStr, "s") {
			stepObjTypeStr += "s"
		}

		if stepObjTypeStr == "views" {
			// This is a view-level change, record the operation
			viewOperations[step.Path] = step.Operation.String()
		} else if isSubResource(step.Type.String()) && strings.HasPrefix(step.Type.String(), "view.") {
			// This is a view sub-resource change
			viewPath := extractTablePathFromSubResource(step.Path, step.Type.String())
			if viewPath != "" {
				// Deduplicate all operations based on (type, operation, path) triplet
				operationKey := step.Path + "." + step.Operation.String() + "." + step.Type.String()
				if !seenOperations[operationKey] {
					seenOperations[operationKey] = true
					subResources[viewPath] = append(subResources[viewPath], struct {
						operation string
						path      string
						subType   string
					}{
						operation: step.Operation.String(),
						path:      step.Path,
						subType:   step.Type.String(),
					})
				}
			}
		}
	}

	// Get all unique view paths (from both direct view changes and sub-resources)
	allViews := make(map[string]bool)
	for viewPath := range viewOperations {
		allViews[viewPath] = true
	}
	for viewPath := range subResources {
		allViews[viewPath] = true
	}

	// Sort view paths for consistent output
	var sortedViews []string
	for viewPath := range allViews {
		sortedViews = append(sortedViews, viewPath)
	}
	sort.Strings(sortedViews)

	// Display each view once with all its changes
	for _, viewPath := range sortedViews {
		var symbol string
		if operation, hasDirectChange := viewOperations[viewPath]; hasDirectChange {
			// View has direct changes, use the operation to determine symbol
			switch operation {
			case "create":
				symbol = c.PlanSymbol("add")
			case "alter":
				symbol = c.PlanSymbol("change")
			case "drop":
				symbol = c.PlanSymbol("destroy")
			default:
				symbol = c.PlanSymbol("change")
			}
		} else {
			// View has no direct changes, only sub-resource changes
			// Sub-resource changes to existing views should always be considered modifications
			symbol = c.PlanSymbol("change")
		}

		fmt.Fprintf(summary, "  %s %s\n", symbol, getLastPathComponent(viewPath))

		// Show sub-resources for this view
		if subResourceList, exists := subResources[viewPath]; exists {
			// Sort sub-resources by type then path
			sort.Slice(subResourceList, func(i, j int) bool {
				if subResourceList[i].subType != subResourceList[j].subType {
					return subResourceList[i].subType < subResourceList[j].subType
				}
				return subResourceList[i].path < subResourceList[j].path
			})

			for _, subRes := range subResourceList {
				var subSymbol string
				switch subRes.operation {
				case "create":
					subSymbol = c.PlanSymbol("add")
				case "alter":
					subSymbol = c.PlanSymbol("change")
				case "drop":
					subSymbol = c.PlanSymbol("destroy")
				default:
					subSymbol = c.PlanSymbol("change")
				}
				// Clean up sub-resource type for display (remove "view." prefix)
				displaySubType := strings.TrimPrefix(subRes.subType, "view.")
				fmt.Fprintf(summary, "    %s %s (%s)\n", subSymbol, getLastPathComponent(subRes.path), displaySubType)
			}
		}
	}
}

// writeMaterializedViewChanges handles materialized view-specific output with proper grouping
func (p *Plan) writeMaterializedViewChanges(summary *strings.Builder, c *color.Color) {
	// Group all changes by materialized view path and track operations
	mvOperations := make(map[string]string) // mv_path -> operation
	subResources := make(map[string][]struct {
		operation string
		path      string
		subType   string
	})

	// Track all seen operations globally to avoid duplicates across groups
	seenOperations := make(map[string]bool) // "path.operation.subType" -> true

	// Track materialized views that have "recreate" operations
	mvsRecreating := make(map[string]bool)

	// Use source diffs for summary calculation
	for _, step := range p.SourceDiffs {
		// Normalize object type
		stepObjTypeStr := step.Type.String()
		if !strings.HasSuffix(stepObjTypeStr, "s") {
			stepObjTypeStr += "s"
		}

		if stepObjTypeStr == "materialized_views" {
			// Track recreate operations so subsequent create is treated as modify
			if step.Operation.String() == "recreate" {
				mvsRecreating[step.Path] = true
			}
			// This is a materialized view-level change, record the operation
			mvOperations[step.Path] = step.Operation.String()
		} else if isSubResource(step.Type.String()) && strings.HasPrefix(step.Type.String(), "materialized_view.") {
			// This is a materialized view sub-resource change
			mvPath := extractTablePathFromSubResource(step.Path, step.Type.String())
			if mvPath != "" {
				// Deduplicate all operations based on (type, operation, path) triplet
				operationKey := step.Path + "." + step.Operation.String() + "." + step.Type.String()
				if !seenOperations[operationKey] {
					seenOperations[operationKey] = true
					subResources[mvPath] = append(subResources[mvPath], struct {
						operation string
						path      string
						subType   string
					}{
						operation: step.Operation.String(),
						path:      step.Path,
						subType:   step.Type.String(),
					})
				}
			}
		}
	}

	// Get all unique materialized view paths (from both direct changes and sub-resources)
	allMVs := make(map[string]bool)
	for mvPath := range mvOperations {
		allMVs[mvPath] = true
	}
	for mvPath := range subResources {
		allMVs[mvPath] = true
	}

	// Sort materialized view paths for consistent output
	var sortedMVs []string
	for mvPath := range allMVs {
		sortedMVs = append(sortedMVs, mvPath)
	}
	sort.Strings(sortedMVs)

	// Display each materialized view once with all its changes
	for _, mvPath := range sortedMVs {
		var symbol string
		if operation, hasDirectChange := mvOperations[mvPath]; hasDirectChange {
			// If this path had a "recreate" and now shows "create", treat as modify
			if mvsRecreating[mvPath] && operation == "create" {
				operation = "alter"
			}
			// Materialized view has direct changes, use the operation to determine symbol
			switch operation {
			case "create":
				symbol = c.PlanSymbol("add")
			case "alter", "recreate":
				// Both "alter" and "recreate" are modifications
				symbol = c.PlanSymbol("change")
			case "drop":
				symbol = c.PlanSymbol("destroy")
			default:
				symbol = c.PlanSymbol("change")
			}
		} else {
			// Materialized view has no direct changes, only sub-resource changes
			// Sub-resource changes to existing materialized views should always be considered modifications
			symbol = c.PlanSymbol("change")
		}

		fmt.Fprintf(summary, "  %s %s\n", symbol, getLastPathComponent(mvPath))

		// Show sub-resources for this materialized view
		if subResourceList, exists := subResources[mvPath]; exists {
			// Sort sub-resources by type then path
			sort.Slice(subResourceList, func(i, j int) bool {
				if subResourceList[i].subType != subResourceList[j].subType {
					return subResourceList[i].subType < subResourceList[j].subType
				}
				return subResourceList[i].path < subResourceList[j].path
			})

			for _, subRes := range subResourceList {
				// Handle online index replacement display
				if subRes.subType == diff.DiffTypeMaterializedViewIndex.String() && subRes.operation == diff.DiffOperationAlter.String() {
					subSymbol := c.PlanSymbol("change")
					displaySubType := strings.TrimPrefix(subRes.subType, "materialized_view.")
					fmt.Fprintf(summary, "    %s %s (%s - concurrent rebuild)\n", subSymbol, getLastPathComponent(subRes.path), displaySubType)
					continue
				}

				var subSymbol string
				switch subRes.operation {
				case "create":
					subSymbol = c.PlanSymbol("add")
				case "alter":
					subSymbol = c.PlanSymbol("change")
				case "drop":
					subSymbol = c.PlanSymbol("destroy")
				default:
					subSymbol = c.PlanSymbol("change")
				}
				// Clean up sub-resource type for display (remove "materialized_view." prefix)
				displaySubType := strings.TrimPrefix(subRes.subType, "materialized_view.")
				fmt.Fprintf(summary, "    %s %s (%s)\n", subSymbol, getLastPathComponent(subRes.path), displaySubType)
			}
		}
	}
}

// writeNonTableChanges handles non-table objects with the original logic
func (p *Plan) writeNonTableChanges(summary *strings.Builder, objType string, c *color.Color) {
	// Collect changes for this object type
	var changes []struct {
		operation string
		path      string
	}

	// Use source diffs for summary calculation
	for _, step := range p.SourceDiffs {
		// Normalize object type
		stepObjTypeStr := step.Type.String()
		if !strings.HasSuffix(stepObjTypeStr, "s") {
			stepObjTypeStr += "s"
		}
		// Normalize underscores to spaces to match Type constants
		stepObjTypeStr = strings.ReplaceAll(stepObjTypeStr, "_", " ")

		if stepObjTypeStr == objType {
			changes = append(changes, struct {
				operation string
				path      string
			}{
				operation: step.Operation.String(),
				path:      step.Path,
			})
		}
	}

	// Sort changes by path for consistent output
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].path < changes[j].path
	})

	// Write changes with appropriate symbols
	for _, change := range changes {
		var symbol string
		switch change.operation {
		case "create":
			symbol = c.PlanSymbol("add")
		case "alter":
			symbol = c.PlanSymbol("change")
		case "drop":
			symbol = c.PlanSymbol("destroy")
		default:
			symbol = c.PlanSymbol("change")
		}

		fmt.Fprintf(summary, "  %s %s\n", symbol, getLastPathComponent(change.path))
	}
}

// isSubResource checks if the given type is a sub-resource of tables, views, or materialized views
func isSubResource(objType string) bool {
	return (strings.HasPrefix(objType, "table.") && objType != "table") ||
		(strings.HasPrefix(objType, "view.") && objType != "view") ||
		(strings.HasPrefix(objType, "materialized_view.") && objType != "materialized_view")
}

// getLastPathComponent extracts the last component from a dot-separated path
func getLastPathComponent(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return path
}

// getObjectNameFromSource extracts the object name from the source object.
// This preserves object names that contain dots (e.g., "public.idx_users")
func getObjectNameFromSource(source diff.DiffSource) string {
	if source == nil {
		return ""
	}
	return source.GetObjectName()
}

// extractTablePathFromSubResource extracts the parent table, view, or materialized view path from a sub-resource path
func extractTablePathFromSubResource(subResourcePath, subResourceType string) string {
	if strings.HasPrefix(subResourceType, "table.") {
		// For sub-resources, the path format depends on the sub-resource type:
		// - "schema.table.resource_name" -> "schema.table" (indexes, policies, columns)
		// - "schema.table" -> "schema.table" (RLS, table comments)
		parts := strings.Split(subResourcePath, ".")

		// Special handling for RLS and table-level changes
		if subResourceType == "table.rls" || subResourceType == "table.comment" {
			// For RLS and table comments, the path is already the table path
			return subResourcePath
		}

		if len(parts) >= 2 {
			// For other sub-resources, return the first two parts as table path
			if len(parts) >= 3 {
				return parts[0] + "." + parts[1]
			}
			// If only 2 parts, it's likely "schema.table" already
			return subResourcePath
		}
	} else if strings.HasPrefix(subResourceType, "view.") {
		// For view sub-resources, the path format is similar:
		// - "schema.view.resource_name" -> "schema.view" (indexes, comments)
		// - "schema.view" -> "schema.view" (view-level comments)
		parts := strings.Split(subResourcePath, ".")

		// Special handling for view-level changes
		if subResourceType == "view.comment" {
			// For view comments, the path is already the view path
			return subResourcePath
		}

		if len(parts) >= 2 {
			// For other sub-resources, return the first two parts as view path
			if len(parts) >= 3 {
				return parts[0] + "." + parts[1]
			}
			// If only 2 parts, it's likely "schema.view" already
			return subResourcePath
		}
	} else if strings.HasPrefix(subResourceType, "materialized_view.") {
		// For materialized view sub-resources, the path format is similar:
		// - "schema.mv.resource_name" -> "schema.mv" (indexes, comments)
		// - "schema.mv" -> "schema.mv" (materialized view-level comments)
		parts := strings.Split(subResourcePath, ".")

		// Special handling for materialized view-level changes
		if subResourceType == "materialized_view.comment" {
			// For materialized view comments, the path is already the materialized view path
			return subResourcePath
		}

		if len(parts) >= 2 {
			// For other sub-resources, return the first two parts as materialized view path
			if len(parts) >= 3 {
				return parts[0] + "." + parts[1]
			}
			// If only 2 parts, it's likely "schema.materialized_view" already
			return subResourcePath
		}
	}
	return ""
}
