package diff

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateViewsSQL generates CREATE VIEW statements
// Views are assumed to be pre-sorted in topological order for dependency-aware creation
func generateCreateViewsSQL(views []*ir.View, targetSchema string, collector *diffCollector) {
	// Process views in the provided order (already topologically sorted)
	for _, view := range views {
		// If compare mode, CREATE OR REPLACE, otherwise CREATE
		sql := generateViewSQL(view, targetSchema)

		// Determine the diff type based on whether it's materialized
		diffType := DiffTypeView
		if view.Materialized {
			diffType = DiffTypeMaterializedView
		}

		// Create context for this statement
		context := &diffContext{
			Type:                diffType,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", view.Schema, view.Name),
			Source:              view,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)

		// Add view comment
		if view.Comment != "" {
			viewName := qualifyEntityName(view.Schema, view.Name, targetSchema)
			commentType := DiffTypeViewComment
			if view.Materialized {
				commentType = DiffTypeMaterializedViewComment
				sql := fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS %s;", viewName, quoteString(view.Comment))

				// Create context for this statement
				context := &diffContext{
					Type:                commentType,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s", view.Schema, view.Name),
					Source:              view,
					CanRunInTransaction: true,
				}

				collector.collect(context, sql)
			} else {
				sql := fmt.Sprintf("COMMENT ON VIEW %s IS %s;", viewName, quoteString(view.Comment))

				// Create context for this statement
				context := &diffContext{
					Type:                commentType,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s", view.Schema, view.Name),
					Source:              view,
					CanRunInTransaction: true,
				}

				collector.collect(context, sql)
			}
		}

		// For materialized views, create indexes
		if view.Materialized && view.Indexes != nil {
			indexList := make([]*ir.Index, 0, len(view.Indexes))
			for _, index := range view.Indexes {
				indexList = append(indexList, index)
			}
			// Generate index SQL for materialized view indexes - use MaterializedView types
			generateCreateIndexesSQLWithType(indexList, targetSchema, collector, DiffTypeMaterializedViewIndex, DiffTypeMaterializedViewIndexComment)
		}

		// Create triggers on this view (e.g., INSTEAD OF triggers)
		if len(view.Triggers) > 0 {
			triggerList := make([]*ir.Trigger, 0, len(view.Triggers))
			for _, trigger := range view.Triggers {
				triggerList = append(triggerList, trigger)
			}
			generateCreateViewTriggersSQL(triggerList, targetSchema, collector)
		}
	}
}

// generateModifyViewsSQL generates CREATE OR REPLACE VIEW statements or comment changes
// preDroppedViews contains views that were already dropped in the pre-drop phase
// dependentViewsCtx contains views that depend on materialized views being recreated
// recreatedViews tracks views that were recreated as dependencies (to avoid duplicate processing)
func generateModifyViewsSQL(diffs []*viewDiff, targetSchema string, collector *diffCollector, preDroppedViews map[string]bool, dependentViewsCtx *dependentViewsContext, recreatedViews map[string]bool) {
	// Track dependent views that have already been dropped to avoid redundant operations
	// when a view depends on multiple materialized views being recreated
	droppedDependentViews := make(map[string]bool)

	// Collect all dependent views that need to be recreated after ALL mat views are processed.
	// This is critical: if a view depends on multiple mat views being recreated, we must
	// wait until ALL mat views are recreated before recreating the dependent view.
	// Otherwise, recreating the view after the first mat view would cause the second
	// mat view's DROP to fail because the recreated view depends on it.
	var allDependentViewsToRecreate []*ir.View
	seenDependentViews := make(map[string]bool)

	// Phase 1: Drop all dependent views and drop/recreate views that require recreation
	for _, diff := range diffs {
		// Handle views that require recreation (DROP + CREATE)
		// This applies to materialized views with structural changes and
		// regular views with column changes incompatible with CREATE OR REPLACE (issue #308)
		if diff.RequiresRecreate {
			viewKey := diff.New.Schema + "." + diff.New.Name
			viewName := qualifyEntityName(diff.New.Schema, diff.New.Name, targetSchema)

			// Determine types based on whether view is materialized
			diffType := DiffTypeView
			if diff.New.Materialized {
				diffType = DiffTypeMaterializedView
			}

			// Get dependent views for this view
			var dependentViews []*ir.View
			if dependentViewsCtx != nil {
				dependentViews = dependentViewsCtx.GetDependents(viewKey)
			}

			// Drop dependent views first (in reverse order to handle nested dependencies).
			// We use RESTRICT (not CASCADE) to fail safely if there are transitive
			// dependencies that we haven't tracked. This prevents silently dropping
			// views that wouldn't be recreated.
			// Skip views that have already been dropped (when a view depends on multiple views).
			for i := len(dependentViews) - 1; i >= 0; i-- {
				depView := dependentViews[i]
				depViewKey := depView.Schema + "." + depView.Name

				// Skip if already dropped (view depends on multiple views being recreated)
				if droppedDependentViews[depViewKey] {
					continue
				}
				droppedDependentViews[depViewKey] = true

				depViewName := qualifyEntityName(depView.Schema, depView.Name, targetSchema)
				dropDepSQL := fmt.Sprintf("DROP VIEW IF EXISTS %s RESTRICT;", depViewName)

				depContext := &diffContext{
					Type:                DiffTypeView,
					Operation:           DiffOperationRecreate,
					Path:                fmt.Sprintf("%s.%s", depView.Schema, depView.Name),
					Source:              depView,
					CanRunInTransaction: true,
				}
				collector.collect(depContext, dropDepSQL)
			}

			// Collect dependent views for later recreation (deduplicated)
			for _, depView := range dependentViews {
				depViewKey := depView.Schema + "." + depView.Name
				if !seenDependentViews[depViewKey] {
					seenDependentViews[depViewKey] = true
					allDependentViewsToRecreate = append(allDependentViewsToRecreate, depView)
				}
			}

			// Check if already pre-dropped
			if preDroppedViews != nil && preDroppedViews[viewKey] {
				// Skip DROP, only CREATE since view was already dropped in pre-drop phase
				createSQL := generateViewSQL(diff.New, targetSchema)

				context := &diffContext{
					Type:                diffType,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
					Source:              diff,
					CanRunInTransaction: true,
				}
				collector.collect(context, createSQL)
			} else {
				// DROP the old view
				var dropSQL string
				if diff.New.Materialized {
					dropSQL = fmt.Sprintf("DROP MATERIALIZED VIEW %s RESTRICT;", viewName)
				} else {
					dropSQL = fmt.Sprintf("DROP VIEW IF EXISTS %s RESTRICT;", viewName)
				}
				createSQL := generateViewSQL(diff.New, targetSchema)

				statements := []SQLStatement{
					{
						SQL:                 dropSQL,
						CanRunInTransaction: true,
					},
					{
						SQL:                 createSQL,
						CanRunInTransaction: true,
					},
				}

				// Use DiffOperationAlter to categorize as a modification
				context := &diffContext{
					Type:                diffType,
					Operation:           DiffOperationAlter,
					Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
					Source:              diff,
					CanRunInTransaction: true,
				}
				collector.collectStatements(context, statements)
			}

			// Add view comment if present
			if diff.New.Comment != "" {
				var commentSQL string
				var commentType DiffType
				if diff.New.Materialized {
					commentSQL = fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS %s;", viewName, quoteString(diff.New.Comment))
					commentType = DiffTypeMaterializedViewComment
				} else {
					commentSQL = fmt.Sprintf("COMMENT ON VIEW %s IS %s;", viewName, quoteString(diff.New.Comment))
					commentType = DiffTypeViewComment
				}
				commentContext := &diffContext{
					Type:                commentType,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
					Source:              diff.New,
					CanRunInTransaction: true,
				}
				collector.collect(commentContext, commentSQL)
			}

			// Recreate indexes for materialized views
			if diff.New.Materialized && diff.New.Indexes != nil {
				indexList := make([]*ir.Index, 0, len(diff.New.Indexes))
				for _, index := range diff.New.Indexes {
					indexList = append(indexList, index)
				}
				generateCreateIndexesSQLWithType(indexList, targetSchema, collector, DiffTypeMaterializedViewIndex, DiffTypeMaterializedViewIndexComment)
			}

			continue // Skip the normal processing for this view
		}

		// Skip views that were already recreated as dependencies of a materialized view
		viewKey := diff.New.Schema + "." + diff.New.Name
		if recreatedViews != nil && recreatedViews[viewKey] {
			continue
		}

		// Check if only the comment changed and definition is identical
		// Both IRs come from pg_get_viewdef() at the same PostgreSQL version, so string comparison is sufficient
		definitionsEqual := diff.Old.Definition == diff.New.Definition
		optionsEqual := viewOptionsEqual(diff.Old.Options, diff.New.Options)
		commentOnlyChange := diff.CommentChanged && definitionsEqual && optionsEqual && diff.Old.Materialized == diff.New.Materialized

		// Check if only indexes changed (for materialized views)
		hasIndexChanges := len(diff.AddedIndexes) > 0 || len(diff.DroppedIndexes) > 0 || len(diff.ModifiedIndexes) > 0
		indexOnlyChange := diff.New.Materialized && hasIndexChanges && definitionsEqual && optionsEqual && !diff.CommentChanged

		// Check if only triggers changed (for INSTEAD OF triggers on views)
		hasTriggerChanges := len(diff.AddedTriggers) > 0 || len(diff.DroppedTriggers) > 0 || len(diff.ModifiedTriggers) > 0
		triggerOnlyChange := hasTriggerChanges && definitionsEqual && optionsEqual && !diff.CommentChanged && !hasIndexChanges

		// Handle non-structural changes (comment-only, index-only, or trigger-only)
		if commentOnlyChange || indexOnlyChange || triggerOnlyChange {
			// Only generate COMMENT ON VIEW statement if comment actually changed
			if diff.CommentChanged {
				viewName := qualifyEntityName(diff.New.Schema, diff.New.Name, targetSchema)

				// Determine the diff type and SQL based on whether it's materialized
				var sql string
				var diffType DiffType
				if diff.New.Materialized {
					diffType = DiffTypeMaterializedView
					if diff.NewComment == "" {
						sql = fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS NULL;", viewName)
					} else {
						sql = fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS %s;", viewName, quoteString(diff.NewComment))
					}
				} else {
					diffType = DiffTypeView
					if diff.NewComment == "" {
						sql = fmt.Sprintf("COMMENT ON VIEW %s IS NULL;", viewName)
					} else {
						sql = fmt.Sprintf("COMMENT ON VIEW %s IS %s;", viewName, quoteString(diff.NewComment))
					}
				}

				// Create context for this statement
				context := &diffContext{
					Type:                diffType,
					Operation:           DiffOperationAlter,
					Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
					Source:              diff,
					CanRunInTransaction: true,
				}

				collector.collect(context, sql)
			}

			// For materialized views, handle index modifications (only if indexes actually changed)
			if diff.New.Materialized && hasIndexChanges {
				generateIndexModifications(
					diff.DroppedIndexes,
					diff.AddedIndexes,
					diff.ModifiedIndexes,
					targetSchema,
					DiffTypeMaterializedViewIndex,
					DiffTypeMaterializedViewIndexComment,
					collector,
				)
			}
		} else {
			// Create the new view (CREATE OR REPLACE works for regular views, materialized views are handled by drop/create cycle)
			sql := generateViewSQL(diff.New, targetSchema)

			// Determine diff type based on whether it's materialized
			diffType := DiffTypeView
			if diff.New.Materialized {
				diffType = DiffTypeMaterializedView
			}

			// Create context for this statement
			context := &diffContext{
				Type:                diffType,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
				Source:              diff,
				CanRunInTransaction: true,
			}

			collector.collect(context, sql)

			// Add view comment for recreated views
			if diff.New.Comment != "" {
				viewName := qualifyEntityName(diff.New.Schema, diff.New.Name, targetSchema)
				var commentSQL string
				var commentType DiffType

				if diff.New.Materialized {
					commentSQL = fmt.Sprintf("COMMENT ON MATERIALIZED VIEW %s IS %s;", viewName, quoteString(diff.New.Comment))
					commentType = DiffTypeMaterializedViewComment
				} else {
					commentSQL = fmt.Sprintf("COMMENT ON VIEW %s IS %s;", viewName, quoteString(diff.New.Comment))
					commentType = DiffTypeViewComment
				}

				// Create context for this statement
				context := &diffContext{
					Type:                commentType,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
					Source:              diff.New,
					CanRunInTransaction: true,
				}

				collector.collect(context, commentSQL)
			}

			// For materialized views that were recreated, recreate indexes
			if diff.New.Materialized && diff.New.Indexes != nil {
				indexList := make([]*ir.Index, 0, len(diff.New.Indexes))
				for _, index := range diff.New.Indexes {
					indexList = append(indexList, index)
				}
				generateCreateIndexesSQLWithType(indexList, targetSchema, collector, DiffTypeMaterializedViewIndex, DiffTypeMaterializedViewIndexComment)
			}
		}

		// Handle trigger changes (e.g., INSTEAD OF triggers) - applies to both branches above
		// Note: DroppedTriggers are skipped here because they are already processed in the DROP phase
		// (see generateDropTriggersFromModifiedViews in trigger.go)
		if len(diff.AddedTriggers) > 0 {
			generateCreateViewTriggersSQL(diff.AddedTriggers, targetSchema, collector)
		}
		for _, triggerDiff := range diff.ModifiedTriggers {
			if triggerDiff.New.IsConstraint {
				viewName := getTableNameWithSchema(diff.New.Schema, diff.New.Name, targetSchema)
				dropSQL := fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s;", triggerDiff.Old.Name, viewName)
				dropContext := &diffContext{
					Type:                DiffTypeViewTrigger,
					Operation:           DiffOperationDrop,
					Path:                fmt.Sprintf("%s.%s.%s", diff.New.Schema, diff.New.Name, triggerDiff.Old.Name),
					Source:              triggerDiff.Old,
					CanRunInTransaction: true,
				}
				collector.collect(dropContext, dropSQL)

				createSQL := generateTriggerSQLWithMode(triggerDiff.New, targetSchema)
				createContext := &diffContext{
					Type:                DiffTypeViewTrigger,
					Operation:           DiffOperationCreate,
					Path:                fmt.Sprintf("%s.%s.%s", diff.New.Schema, diff.New.Name, triggerDiff.New.Name),
					Source:              triggerDiff.New,
					CanRunInTransaction: true,
				}
				collector.collect(createContext, createSQL)
			} else {
				sql := generateTriggerSQLWithMode(triggerDiff.New, targetSchema)
				context := &diffContext{
					Type:                DiffTypeViewTrigger,
					Operation:           DiffOperationAlter,
					Path:                fmt.Sprintf("%s.%s.%s", diff.New.Schema, diff.New.Name, triggerDiff.New.Name),
					Source:              triggerDiff.New,
					CanRunInTransaction: true,
				}
				collector.collect(context, sql)
			}
		}
	}

	// Phase 2: Recreate all dependent views AFTER all materialized views have been processed.
	// This is critical for views that depend on multiple mat views being recreated.
	// Re-sort topologically to ensure correct order when views from different mat view
	// dependency lists have cross-dependencies (e.g., V3 from mat_B depends on V1 from mat_A).
	sortedDependentViews := topologicallySortViews(allDependentViewsToRecreate)
	for _, depView := range sortedDependentViews {
		depViewKey := depView.Schema + "." + depView.Name

		// Skip if already recreated by other means
		if recreatedViews != nil && recreatedViews[depViewKey] {
			continue
		}

		createDepSQL := generateViewSQL(depView, targetSchema)

		depContext := &diffContext{
			Type:                DiffTypeView,
			Operation:           DiffOperationRecreate,
			Path:                fmt.Sprintf("%s.%s", depView.Schema, depView.Name),
			Source:              depView,
			CanRunInTransaction: true,
		}
		collector.collect(depContext, createDepSQL)

		// Track this view as recreated to avoid duplicate processing
		if recreatedViews != nil {
			recreatedViews[depViewKey] = true
		}

		// Recreate view comment if present
		if depView.Comment != "" {
			depViewName := qualifyEntityName(depView.Schema, depView.Name, targetSchema)
			commentSQL := fmt.Sprintf("COMMENT ON VIEW %s IS %s;", depViewName, quoteString(depView.Comment))
			commentContext := &diffContext{
				Type:                DiffTypeViewComment,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s", depView.Schema, depView.Name),
				Source:              depView,
				CanRunInTransaction: true,
			}
			collector.collect(commentContext, commentSQL)
		}
	}
}

// generateDropViewsSQL generates DROP [MATERIALIZED] VIEW statements
// Views are assumed to be pre-sorted in reverse topological order for dependency-aware dropping
func generateDropViewsSQL(views []*ir.View, targetSchema string, collector *diffCollector) {
	// Process views in the provided order (already reverse topologically sorted)
	for _, view := range views {
		viewName := qualifyEntityName(view.Schema, view.Name, targetSchema)
		var sql string
		var diffType DiffType
		if view.Materialized {
			sql = fmt.Sprintf("DROP MATERIALIZED VIEW %s RESTRICT;", viewName)
			diffType = DiffTypeMaterializedView
		} else {
			sql = fmt.Sprintf("DROP VIEW IF EXISTS %s CASCADE;", viewName)
			diffType = DiffTypeView
		}

		// Create context for this statement
		context := &diffContext{
			Type:                diffType,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", view.Schema, view.Name),
			Source:              view,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateViewSQL generates CREATE [OR REPLACE] [MATERIALIZED] VIEW statement
func generateViewSQL(view *ir.View, targetSchema string) string {
	// Determine view name based on context
	var viewName string
	if targetSchema != "" {
		// For diff scenarios, use schema qualification logic
		viewName = qualifyEntityName(view.Schema, view.Name, targetSchema)
	} else {
		// For dump scenarios, always include schema prefix
		viewName = fmt.Sprintf("%s.%s", view.Schema, view.Name)
	}

	// Determine CREATE statement type
	var createClause string
	if view.Materialized {
		createClause = "CREATE MATERIALIZED VIEW IF NOT EXISTS"
	} else {
		createClause = "CREATE OR REPLACE VIEW"
	}

	// Add WITH clause for view options (e.g., security_invoker, security_barrier)
	var withClause string
	if len(view.Options) > 0 {
		withClause = " WITH (" + strings.Join(view.Options, ", ") + ")"
	}

	// Use the view definition as-is - it has already been normalized
	return fmt.Sprintf("%s %s%s AS\n%s;", createClause, viewName, withClause, view.Definition)
}

// diffViewTriggers computes added, dropped, and modified triggers between two views
func diffViewTriggers(oldView, newView *ir.View) ([]*ir.Trigger, []*ir.Trigger, []*triggerDiff) {
	oldTriggers := oldView.Triggers
	newTriggers := newView.Triggers
	if oldTriggers == nil {
		oldTriggers = make(map[string]*ir.Trigger)
	}
	if newTriggers == nil {
		newTriggers = make(map[string]*ir.Trigger)
	}

	var added []*ir.Trigger
	var dropped []*ir.Trigger
	var modified []*triggerDiff

	for name, trigger := range newTriggers {
		if _, exists := oldTriggers[name]; !exists {
			added = append(added, trigger)
		}
	}
	for name, trigger := range oldTriggers {
		if _, exists := newTriggers[name]; !exists {
			dropped = append(dropped, trigger)
		}
	}
	for name, newTrigger := range newTriggers {
		if oldTrigger, exists := oldTriggers[name]; exists {
			if !triggersEqual(oldTrigger, newTrigger) {
				modified = append(modified, &triggerDiff{
					Old: oldTrigger,
					New: newTrigger,
				})
			}
		}
	}

	// Sort for deterministic output (Go map iteration is random)
	sort.Slice(added, func(i, j int) bool {
		return added[i].Name < added[j].Name
	})
	sort.Slice(dropped, func(i, j int) bool {
		return dropped[i].Name < dropped[j].Name
	})
	sort.Slice(modified, func(i, j int) bool {
		return modified[i].Old.Name < modified[j].Old.Name
	})

	return added, dropped, modified
}

// viewsEqual compares two views for equality
// Both IRs come from pg_get_viewdef() at the same PostgreSQL version, so string comparison is sufficient
func viewsEqual(old, new *ir.View) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}

	// Check if materialized status differs
	if old.Materialized != new.Materialized {
		return false
	}

	// Compare view options (e.g., security_invoker, security_barrier)
	if !viewOptionsEqual(old.Options, new.Options) {
		return false
	}

	// Both definitions come from pg_get_viewdef(), so they are already normalized
	return old.Definition == new.Definition
}

// viewColumnsRequireRecreate checks whether the view's column set has changed
// in a way that requires DROP + CREATE instead of CREATE OR REPLACE.
// PostgreSQL's CREATE OR REPLACE VIEW only allows adding new columns at the end;
// it rejects any changes to existing column names or positions.
func viewColumnsRequireRecreate(old, new *ir.View) bool {
	oldCols := old.Columns
	newCols := new.Columns

	// If column info is not available, fall back to safe behavior (no recreate needed;
	// CREATE OR REPLACE will fail at apply time if columns are incompatible)
	if len(oldCols) == 0 || len(newCols) == 0 {
		return false
	}

	// If old columns are a prefix of new columns, CREATE OR REPLACE VIEW is safe
	// (only new columns added at end)
	if len(newCols) >= len(oldCols) {
		prefixMatch := true
		for i, col := range oldCols {
			if newCols[i] != col {
				prefixMatch = false
				break
			}
		}
		if prefixMatch {
			return false
		}
	}

	// Columns were reordered, renamed, or removed — requires DROP + CREATE
	return true
}

// viewDependsOnView checks if viewA depends on viewB
func viewDependsOnView(viewA *ir.View, viewBName string) bool {
	if viewA == nil || viewA.Definition == "" {
		return false
	}
	return containsIdentifier(viewA.Definition, viewBName)
}

// containsIdentifier checks if the given SQL text contains the identifier as a whole word.
// This uses word boundary matching to avoid false positives (e.g., "user" matching "users").
func containsIdentifier(sqlText, identifier string) bool {
	if sqlText == "" || identifier == "" {
		return false
	}

	// Build a regex pattern that matches the identifier as a whole word.
	// Word boundaries in SQL are: start/end of string, whitespace, punctuation, operators.
	// We use a pattern that matches the identifier not preceded/followed by word characters.
	//
	// For schema-qualified identifiers (containing a dot), treat '.' as part of the word
	// to avoid matching inside longer qualified paths like "other.schema.name".
	// Use [^a-zA-Z0-9_] to exclude both upper and lowercase letters as word boundaries.
	var pattern string
	if strings.Contains(identifier, ".") {
		pattern = `(?i)(?:^|[^a-zA-Z0-9_.])` + regexp.QuoteMeta(identifier) + `(?:[^a-zA-Z0-9_.]|$)`
	} else {
		pattern = `(?i)(?:^|[^a-zA-Z0-9_])` + regexp.QuoteMeta(identifier) + `(?:[^a-zA-Z0-9_]|$)`
	}
	matched, err := regexp.MatchString(pattern, sqlText)
	if err != nil {
		// This should never happen since regexp.QuoteMeta ensures valid pattern,
		// but log it rather than silently ignoring
		fmt.Printf("containsIdentifier: regexp error for pattern %q: %v\n", pattern, err)
		return false
	}
	return matched
}

// viewDependsOnTable checks if a view depends on a specific table
// by checking if the table name appears in the view definition
func viewDependsOnTable(view *ir.View, tableSchema, tableName string) bool {
	if view == nil || view.Definition == "" {
		return false
	}

	def := strings.ToLower(view.Definition)
	tableNameLower := strings.ToLower(tableName)

	// Check for unqualified table name
	if strings.Contains(def, tableNameLower) {
		return true
	}

	// Check for qualified table name (schema.table)
	qualifiedName := strings.ToLower(tableSchema + "." + tableName)
	if strings.Contains(def, qualifiedName) {
		return true
	}

	return false
}

// dependentViewsContext tracks views that depend on views being recreated
type dependentViewsContext struct {
	// dependents maps view key (schema.name) to list of dependent views
	dependents map[string][]*ir.View
}

// newDependentViewsContext creates a new context for tracking dependent views
func newDependentViewsContext() *dependentViewsContext {
	return &dependentViewsContext{
		dependents: make(map[string][]*ir.View),
	}
}

// GetDependents returns the list of views that depend on the given view being recreated
func (ctx *dependentViewsContext) GetDependents(viewKey string) []*ir.View {
	if ctx == nil || ctx.dependents == nil {
		return nil
	}
	return ctx.dependents[viewKey]
}

// findDependentViewsForRecreatedViews finds all views that depend on views being recreated.
// This includes transitive dependencies (views that depend on views that depend on the recreated view).
// Handles both materialized views and regular views with RequiresRecreate (issue #308).
// allViews contains all views from the new state (used for dependency analysis and recreation)
// modifiedViews contains the views being recreated
// addedViews contains views that are newly added (not in old schema) - these should be excluded
// because they will be created in the CREATE phase after recreated views
func findDependentViewsForRecreatedViews(allViews map[string]*ir.View, modifiedViews []*viewDiff, addedViews []*ir.View) *dependentViewsContext {
	ctx := newDependentViewsContext()

	// Build a set of added view keys to exclude from dependents
	addedViewKeys := make(map[string]bool)
	for _, view := range addedViews {
		addedViewKeys[view.Schema+"."+view.Name] = true
	}

	for _, viewDiff := range modifiedViews {
		if !viewDiff.RequiresRecreate {
			continue
		}

		recreatedViewKey := viewDiff.New.Schema + "." + viewDiff.New.Name

		// Find all views that directly depend on this view being recreated
		// Exclude newly added views - they will be created in CREATE phase
		directDependents := make([]*ir.View, 0)
		for _, view := range allViews {
			viewKey := view.Schema + "." + view.Name
			if addedViewKeys[viewKey] {
				continue // Skip newly added views
			}
			// Skip the view being recreated itself
			if viewKey == recreatedViewKey {
				continue
			}
			if viewDependsOnView(view, viewDiff.New.Name) ||
				viewDependsOnView(view, viewDiff.New.Schema+"."+viewDiff.New.Name) {
				directDependents = append(directDependents, view)
			}
		}

		// Find transitive dependencies (views that depend on the direct dependents)
		// Also exclude added views from transitive search
		allDependents := findTransitiveDependents(directDependents, allViews, addedViewKeys)

		// Topologically sort the dependents so they can be dropped/recreated in correct order
		sortedDependents := topologicallySortViews(allDependents)

		ctx.dependents[recreatedViewKey] = sortedDependents
	}

	return ctx
}

// findTransitiveDependents finds all views that transitively depend on the given views.
// Returns all dependents including the initial views, with no duplicates.
// excludeKeys contains view keys to exclude (e.g., newly added views)
func findTransitiveDependents(initialViews []*ir.View, allViews map[string]*ir.View, excludeKeys map[string]bool) []*ir.View {
	if len(initialViews) == 0 {
		return nil
	}

	// Track visited views to avoid duplicates and cycles
	visited := make(map[string]bool)
	var result []*ir.View

	// Queue for BFS traversal
	queue := make([]*ir.View, 0, len(initialViews))
	for _, v := range initialViews {
		key := v.Schema + "." + v.Name
		if !visited[key] {
			visited[key] = true
			queue = append(queue, v)
			result = append(result, v)
		}
	}

	// BFS to find all transitive dependents
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Find views that depend on the current view
		for _, view := range allViews {
			if view.Materialized {
				continue // Skip materialized views
			}
			viewKey := view.Schema + "." + view.Name
			if visited[viewKey] {
				continue
			}
			if excludeKeys != nil && excludeKeys[viewKey] {
				continue // Skip excluded views (e.g., newly added views)
			}

			// Check if this view depends on the current view (unqualified or schema-qualified)
			if viewDependsOnView(view, current.Name) ||
				viewDependsOnView(view, current.Schema+"."+current.Name) {
				visited[viewKey] = true
				queue = append(queue, view)
				result = append(result, view)
			}
		}
	}

	return result
}

// sortModifiedViewsForProcessing sorts modifiedViews to ensure views
// with RequiresRecreate are processed first. This ensures dependent views are
// added to recreatedViews before their own modifications would be processed.
func sortModifiedViewsForProcessing(views []*viewDiff) {
	sort.SliceStable(views, func(i, j int) bool {
		// Views with RequiresRecreate should come first
		iRecreate := views[i].RequiresRecreate
		jRecreate := views[j].RequiresRecreate

		if iRecreate && !jRecreate {
			return true
		}
		if !iRecreate && jRecreate {
			return false
		}

		// Otherwise maintain relative order (stable sort)
		return false
	})
}

// viewOptionsEqual compares two view option slices for equality
func viewOptionsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, opt := range a {
		if opt != b[i] {
			return false
		}
	}
	return true
}
