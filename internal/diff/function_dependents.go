package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// A function change CREATE OR REPLACE cannot apply (return type, parameter
// names, OUT parameters) is planned as DROP FUNCTION + CREATE FUNCTION (#326).
// Calling views go through that cycle with the function (#601, see
// splitFunctionsRecreatedUnderViews). The other objects PostgreSQL records a
// dependency for are handled here: column defaults, CHECK and EXCLUDE
// constraints, expression and partial indexes, row-level security policies,
// trigger WHEN conditions, and domain defaults and CHECK constraints.
//
// Such an object is held when its desired definition calls a recreated
// function: its current version, if any, is dropped right before the
// function and the desired version is created once the function exists
// again, so both land in the transaction that drops and creates the function
// (only a later VALIDATE CONSTRAINT or a brand-new CONCURRENTLY index runs on
// its own). The regular table and domain diff leaves it alone, except for the
// early drop of an EXCLUDE constraint, policy or trigger naming a column it
// re-creates (#591). An object whose current definition calls the function
// but whose desired one does not is left to the regular diff, which drops or
// replaces it in the modify phase, still ahead of the function's DROP.
//
// Matching is by function name (see referencesNewFunction), so an overload of
// a recreated function causes a redundant drop and restore of its callers.
//
// Generated columns and columns added (or re-created) with a DEFAULT that
// calls a recreated function are not carried through the cycle;
// ValidateFunctionRecreations rejects those plans.

// heldTableDependents are the objects of one table held around the recreation
// of the functions they call.
type heldTableDependents struct {
	table    *ir.Table // desired state
	existing bool      // the table exists in the current state

	droppedDefaults    []*ir.Column // current columns whose default calls a recreated function
	defaults           []*ir.Column // desired columns whose default calls a recreated function
	droppedConstraints []*ir.Constraint
	constraints        []*ir.Constraint
	droppedIndexes     []*ir.Index
	indexes            []*ir.Index
	droppedPolicies    []*ir.RLSPolicy
	policies           []*ir.RLSPolicy
	droppedTriggers    []*ir.Trigger
	triggers           []*ir.Trigger
}

func (h *heldTableDependents) empty() bool {
	return len(h.defaults) == 0 && len(h.constraints) == 0 && len(h.indexes) == 0 &&
		len(h.policies) == 0 && len(h.triggers) == 0
}

// heldDomainDependents are the default and CHECK constraints of one domain
// held around the recreation of the functions they call.
type heldDomainDependents struct {
	domain             *ir.Type // desired state
	dropDefault        bool     // the current default calls a recreated function
	setDefault         bool     // the desired default calls a recreated function
	droppedConstraints []*ir.DomainConstraint
	constraints        []*ir.DomainConstraint
}

// holdRecreatedFunctionDependents collects the objects whose desired
// definition calls a function this migration drops and creates again, takes
// them out of the regular table and domain diff, and strips them from the
// tables and domains the create phase adds. (#601)
func (d *ddlDiff) holdRecreatedFunctionDependents(oldTables, newTables map[string]*ir.Table, oldTypes, newTypes map[string]*ir.Type) {
	recreated := buildRoutineLookup(recreatedFunctions(d.modifiedFunctions), nil)
	if len(recreated) == 0 {
		return
	}

	modifiedTables := make(map[string]*tableDiff, len(d.modifiedTables))
	for _, td := range d.modifiedTables {
		modifiedTables[td.Table.Schema+"."+td.Table.Name] = td
	}
	addedTables := make(map[string]int, len(d.addedTables))
	for i, table := range d.addedTables {
		addedTables[table.Schema+"."+table.Name] = i
	}

	for _, key := range sortedKeys(newTables) {
		newTable := newTables[key]
		oldTable := oldTables[key]
		if newTable.IsExternal || (oldTable != nil && oldTable.IsExternal) {
			continue
		}
		td := modifiedTables[key]
		h := collectHeldTableDependents(oldTable, newTable, partitionParent(oldTables, oldTable), partitionParent(newTables, newTable), td, recreated)
		if h == nil {
			continue
		}
		if h.existing {
			if td != nil {
				td.releaseHeldDependents(h)
			}
		} else if i, ok := addedTables[key]; ok {
			d.addedTables[i] = stripHeldDependents(newTable, h)
		}
		d.heldTableDependents = append(d.heldTableDependents, h)
	}

	modifiedTypes := make(map[string]*typeDiff, len(d.modifiedTypes))
	for _, td := range d.modifiedTypes {
		modifiedTypes[td.New.Schema+"."+td.New.Name] = td
	}
	addedTypes := make(map[string]int, len(d.addedTypes))
	for i, typ := range d.addedTypes {
		addedTypes[typ.Schema+"."+typ.Name] = i
	}

	for _, key := range sortedKeys(newTypes) {
		newDomain := newTypes[key]
		if newDomain.Kind != ir.TypeKindDomain {
			continue
		}
		oldDomain := oldTypes[key]
		if oldDomain != nil && oldDomain.Kind != ir.TypeKindDomain {
			continue
		}
		h := collectHeldDomainDependents(oldDomain, newDomain, recreated)
		if h == nil {
			continue
		}
		if oldDomain != nil {
			if td := modifiedTypes[key]; td != nil {
				// A held default is left as it is by the regular diff.
				newDefault := td.New.Default
				if h.setDefault {
					newDefault = td.Old.Default
				}
				td.Old = withoutHeldDomainDependents(td.Old, h, td.Old.Default)
				td.New = withoutHeldDomainDependents(td.New, h, newDefault)
			}
		} else if i, ok := addedTypes[key]; ok {
			newDefault := newDomain.Default
			if h.setDefault {
				newDefault = ""
			}
			d.addedTypes[i] = withoutHeldDomainDependents(newDomain, h, newDefault)
		}
		d.heldDomainDependents = append(d.heldDomainDependents, h)
	}
}

// partitionParent returns the partitioned table a partition belongs to, if it
// is part of the same state.
func partitionParent(tables map[string]*ir.Table, table *ir.Table) *ir.Table {
	if table == nil || table.PartitionOf == "" {
		return nil
	}
	schema := table.PartitionOfSchema
	if schema == "" {
		schema = table.Schema
	}
	return tables[schema+"."+table.PartitionOf]
}

// inheritedConstraint and inheritedIndex report whether a partition's constraint or index is
// the copy PostgreSQL keeps for a CHECK constraint or index of its
// partitioned parent. Those go with the parent's: dropping them on their own
// fails, and adding the parent's creates them.
func inheritedConstraint(parent *ir.Table, constraint *ir.Constraint) bool {
	return parent != nil && parent.Constraints[constraint.Name] != nil
}

func inheritedIndex(parent *ir.Table, index *ir.Index) bool {
	if parent == nil {
		return false
	}
	for _, parentIndex := range parent.Indexes {
		if indexesStructurallyEqual(parentIndex, index) {
			return true
		}
	}
	return false
}

// collectHeldTableDependents returns the objects of newTable whose desired
// definition calls a recreated function, with the current versions to drop
// first, or nil if there are none. For a partition, the constraints and
// indexes it inherits from its parent are left to the parent's.
func collectHeldTableDependents(oldTable, newTable, oldParent, newParent *ir.Table, td *tableDiff, recreated map[string]struct{}) *heldTableDependents {
	h := &heldTableDependents{table: newTable, existing: oldTable != nil}
	calls := func(expr string) bool { return referencesNewFunction(expr, newTable.Schema, recreated) }

	var recreatedColumns map[string]bool
	if td != nil {
		recreatedColumns = td.RecreatedColumns
	}
	oldColumns := make(map[string]*ir.Column)
	if oldTable != nil {
		for _, col := range oldTable.Columns {
			oldColumns[col.Name] = col
		}
	}
	for _, col := range newTable.Columns {
		if col.DefaultValue == nil || !calls(*col.DefaultValue) {
			continue
		}
		if oldTable == nil {
			h.defaults = append(h.defaults, col)
			continue
		}
		oldCol := oldColumns[col.Name]
		// A column added or re-created (#591) by this migration fills its
		// existing rows from the default; ValidateFunctionRecreations refuses
		// that.
		if oldCol == nil || recreatedColumns[col.Name] {
			continue
		}
		h.defaults = append(h.defaults, col)
		if oldCol.DefaultValue != nil && calls(*oldCol.DefaultValue) {
			h.droppedDefaults = append(h.droppedDefaults, oldCol)
		}
	}

	for _, name := range sortedKeys(newTable.Constraints) {
		constraint := newTable.Constraints[name]
		if !constraintCallsFunction(constraint, recreated) || inheritedConstraint(newParent, constraint) {
			continue
		}
		h.constraints = append(h.constraints, constraint)
		if oldTable != nil && oldTable.Constraints[name] != nil && !inheritedConstraint(oldParent, oldTable.Constraints[name]) {
			h.droppedConstraints = append(h.droppedConstraints, oldTable.Constraints[name])
		}
	}

	for _, name := range sortedKeys(newTable.Indexes) {
		index := newTable.Indexes[name]
		if !indexCallsFunction(index, recreated) || inheritedIndex(newParent, index) {
			continue
		}
		h.indexes = append(h.indexes, index)
		if oldTable != nil && oldTable.Indexes[name] != nil && !inheritedIndex(oldParent, oldTable.Indexes[name]) {
			h.droppedIndexes = append(h.droppedIndexes, oldTable.Indexes[name])
		}
	}

	for _, name := range sortedKeys(newTable.Policies) {
		policy := newTable.Policies[name]
		if !policyReferencesNewFunction(policy, recreated) {
			continue
		}
		h.policies = append(h.policies, policy)
		if oldTable != nil && oldTable.Policies[name] != nil {
			h.droppedPolicies = append(h.droppedPolicies, oldTable.Policies[name])
		}
	}

	for _, name := range sortedKeys(newTable.Triggers) {
		trigger := newTable.Triggers[name]
		if !calls(trigger.Condition) {
			continue
		}
		h.triggers = append(h.triggers, trigger)
		if oldTable != nil && oldTable.Triggers[name] != nil {
			h.droppedTriggers = append(h.droppedTriggers, oldTable.Triggers[name])
		}
	}

	if h.empty() {
		return nil
	}
	sort.SliceStable(h.constraints, func(i, j int) bool {
		return constraintTypeOrder(h.constraints[i]) < constraintTypeOrder(h.constraints[j])
	})
	return h
}

// constraintCallsFunction reports whether a CHECK or EXCLUDE constraint calls
// a function in the lookup; other constraint types hold no expressions.
func constraintCallsFunction(constraint *ir.Constraint, functions map[string]struct{}) bool {
	switch constraint.Type {
	case ir.ConstraintTypeCheck:
		return referencesNewFunction(constraint.CheckClause, constraint.Schema, functions)
	case ir.ConstraintTypeExclusion:
		return referencesNewFunction(constraint.ExclusionDefinition, constraint.Schema, functions)
	}
	return false
}

// indexCallsFunction reports whether an index calls a function in the lookup
// in an expression column or its partial-index predicate.
func indexCallsFunction(index *ir.Index, functions map[string]struct{}) bool {
	if index.IsExpression {
		for _, col := range index.Columns {
			if referencesNewFunction(col.Name, index.Schema, functions) {
				return true
			}
		}
	}
	return index.IsPartial && referencesNewFunction(index.Where, index.Schema, functions)
}

// releaseHeldDependents takes the held objects out of the regular table diff:
// they are dropped right before the functions they call and created again
// after them. An EXCLUDE constraint, policy or trigger naming a column the
// regular diff re-creates (#591) must be gone before that column's DROP, so
// the regular diff keeps dropping it.
func (td *tableDiff) releaseHeldDependents(h *heldTableDependents) {
	constraints := make(map[string]bool)
	for _, c := range h.constraints {
		constraints[c.Name] = true
	}
	td.AddedConstraints = removeByName(td.AddedConstraints, constraints, func(c *ir.Constraint) string { return c.Name })
	td.ModifiedConstraints = removeByName(td.ModifiedConstraints, constraints, func(c *ConstraintDiff) string { return c.New.Name })
	earlyConstraints := make(map[string]bool)
	for _, c := range td.DroppedConstraints {
		if constraints[c.Name] && exclusionReferencesColumns(c, td.RecreatedColumns) {
			earlyConstraints[c.Name] = true
		}
	}
	td.DroppedConstraints = removeByName(td.DroppedConstraints, subtract(constraints, earlyConstraints), func(c *ir.Constraint) string { return c.Name })
	h.droppedConstraints = removeByName(h.droppedConstraints, earlyConstraints, func(c *ir.Constraint) string { return c.Name })

	indexes := make(map[string]bool)
	for _, index := range h.indexes {
		indexes[index.Name] = true
	}
	td.AddedIndexes = removeByName(td.AddedIndexes, indexes, func(i *ir.Index) string { return i.Name })
	td.DroppedIndexes = removeByName(td.DroppedIndexes, indexes, func(i *ir.Index) string { return i.Name })
	td.ModifiedIndexes = removeByName(td.ModifiedIndexes, indexes, func(i *IndexDiff) string { return i.New.Name })

	policies := make(map[string]bool)
	for _, policy := range h.policies {
		policies[policy.Name] = true
	}
	td.AddedPolicies = removeByName(td.AddedPolicies, policies, func(p *ir.RLSPolicy) string { return p.Name })
	td.ModifiedPolicies = removeByName(td.ModifiedPolicies, policies, func(p *policyDiff) string { return p.New.Name })
	earlyPolicies := make(map[string]bool)
	for _, p := range td.DroppedPolicies {
		if policies[p.Name] && policyReferencesColumns(p, td.RecreatedColumns) {
			earlyPolicies[p.Name] = true
		}
	}
	td.DroppedPolicies = removeByName(td.DroppedPolicies, subtract(policies, earlyPolicies), func(p *ir.RLSPolicy) string { return p.Name })
	h.droppedPolicies = removeByName(h.droppedPolicies, earlyPolicies, func(p *ir.RLSPolicy) string { return p.Name })

	triggers := make(map[string]bool)
	for _, trigger := range h.triggers {
		triggers[trigger.Name] = true
	}
	td.AddedTriggers = removeByName(td.AddedTriggers, triggers, func(t *ir.Trigger) string { return t.Name })
	td.ModifiedTriggers = removeByName(td.ModifiedTriggers, triggers, func(t *triggerDiff) string { return t.New.Name })
	earlyTriggers := make(map[string]bool)
	for _, t := range td.DroppedTriggers {
		if triggers[t.Name] && triggerReferencesColumns(t, td.RecreatedColumns) {
			earlyTriggers[t.Name] = true
		}
	}
	td.DroppedTriggers = removeByName(td.DroppedTriggers, subtract(triggers, earlyTriggers), func(t *ir.Trigger) string { return t.Name })
	h.droppedTriggers = removeByName(h.droppedTriggers, earlyTriggers, func(t *ir.Trigger) string { return t.Name })

	// The regular column diff leaves a held default as it is; it is dropped
	// and set again around the function. (A held default on a column whose
	// type changes is refused by ValidateFunctionRecreations.)
	heldDefaults := make(map[string]bool)
	for _, col := range h.defaults {
		heldDefaults[col.Name] = true
	}
	for _, cd := range td.ModifiedColumns {
		if heldDefaults[cd.New.Name] {
			cd.New = columnWithDefault(cd.New, cd.Old.DefaultValue)
		}
	}
}

// subtract returns the names in a that are not in b.
func subtract(a, b map[string]bool) map[string]bool {
	if len(b) == 0 {
		return a
	}
	rest := make(map[string]bool, len(a))
	for name := range a {
		if !b[name] {
			rest[name] = true
		}
	}
	return rest
}

// onlyTable returns the name to use in ALTER TABLE for a change that must not
// recurse to the partitions of a partitioned table.
func onlyTable(table *ir.Table, tableName string) string {
	if table.IsPartitioned {
		return "ONLY " + tableName
	}
	return tableName
}

func removeByName[T any](items []T, names map[string]bool, name func(T) string) []T {
	if len(names) == 0 {
		return items
	}
	kept := items[:0:0]
	for _, item := range items {
		if !names[name(item)] {
			kept = append(kept, item)
		}
	}
	return kept
}

// columnWithDefault returns a copy of col with the given default.
func columnWithDefault(col *ir.Column, value *string) *ir.Column {
	c := *col
	c.DefaultValue = value
	return &c
}

// stripHeldDependents returns a copy of a table the create phase adds,
// without the held objects: they are created after the functions they call.
// The table is empty then, so a default set afterwards fills no rows.
func stripHeldDependents(table *ir.Table, h *heldTableDependents) *ir.Table {
	stripped := *table
	heldDefaults := make(map[string]bool)
	for _, col := range h.defaults {
		heldDefaults[col.Name] = true
	}
	stripped.Columns = make([]*ir.Column, len(table.Columns))
	for i, col := range table.Columns {
		if heldDefaults[col.Name] {
			col = columnWithDefault(col, nil)
		}
		stripped.Columns[i] = col
	}
	stripped.Constraints = withoutKeys(table.Constraints, h.constraints, func(c *ir.Constraint) string { return c.Name })
	stripped.Indexes = withoutKeys(table.Indexes, h.indexes, func(i *ir.Index) string { return i.Name })
	stripped.Policies = withoutKeys(table.Policies, h.policies, func(p *ir.RLSPolicy) string { return p.Name })
	stripped.Triggers = withoutKeys(table.Triggers, h.triggers, func(t *ir.Trigger) string { return t.Name })
	return &stripped
}

func withoutKeys[T any](m map[string]T, held []T, name func(T) string) map[string]T {
	if len(held) == 0 {
		return m
	}
	skip := make(map[string]bool, len(held))
	for _, item := range held {
		skip[name(item)] = true
	}
	kept := make(map[string]T, len(m))
	for key, value := range m {
		if !skip[key] {
			kept[key] = value
		}
	}
	return kept
}

// collectHeldDomainDependents returns the default and CHECK constraints of
// newDomain that call a recreated function, or nil if there are none. Only
// named constraints can be dropped and added again.
func collectHeldDomainDependents(oldDomain, newDomain *ir.Type, recreated map[string]struct{}) *heldDomainDependents {
	h := &heldDomainDependents{domain: newDomain}
	calls := func(expr string) bool { return referencesNewFunction(expr, newDomain.Schema, recreated) }

	if calls(newDomain.Default) {
		h.setDefault = true
		h.dropDefault = oldDomain != nil && calls(oldDomain.Default)
	}
	for _, constraint := range newDomain.Constraints {
		if constraint.Name == "" || !calls(constraint.Definition) {
			continue
		}
		h.constraints = append(h.constraints, constraint)
		if oldDomain == nil {
			continue
		}
		for _, old := range oldDomain.Constraints {
			if old.Name == constraint.Name {
				h.droppedConstraints = append(h.droppedConstraints, old)
			}
		}
	}
	if !h.setDefault && len(h.constraints) == 0 {
		return nil
	}
	sort.Slice(h.constraints, func(i, j int) bool { return h.constraints[i].Name < h.constraints[j].Name })
	sort.Slice(h.droppedConstraints, func(i, j int) bool { return h.droppedConstraints[i].Name < h.droppedConstraints[j].Name })
	return h
}

// withoutHeldDomainDependents returns a copy of a domain without its held
// constraints and with the given default.
func withoutHeldDomainDependents(domain *ir.Type, h *heldDomainDependents, defaultValue string) *ir.Type {
	stripped := *domain
	held := make(map[string]bool, len(h.constraints))
	for _, c := range h.constraints {
		held[c.Name] = true
	}
	stripped.Constraints = nil
	for _, c := range domain.Constraints {
		if !held[c.Name] {
			stripped.Constraints = append(stripped.Constraints, c)
		}
	}
	stripped.Default = defaultValue
	return &stripped
}

// generateDropHeldFunctionDependentsSQL drops the current version of the held
// objects right before the functions they call are dropped. A dropped
// constraint, index, policy or trigger is recorded as a recreate operation.
// The regular diff ran before and may have dropped a constraint or index
// already with a column it depends on, hence IF EXISTS. A default on a
// partitioned table is dropped and set with ONLY: without it PostgreSQL
// applies the change to every partition, overwriting partition defaults of
// their own. (#601)
func (d *ddlDiff) generateDropHeldFunctionDependentsSQL(targetSchema string, collector *diffCollector) {
	defer markHeldDependents(collector, len(collector.diffs))
	for _, h := range d.heldDomainDependents {
		domainName := qualifyEntityName(h.domain.Schema, h.domain.Name, targetSchema)
		var statements []string
		if h.dropDefault {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP DEFAULT;", domainName))
		}
		for _, c := range h.droppedConstraints {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP CONSTRAINT IF EXISTS %s;", domainName, ir.QuoteIdentifier(c.Name)))
		}
		for _, stmt := range statements {
			collector.collect(&diffContext{
				Type:                DiffTypeDomain,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", h.domain.Schema, h.domain.Name),
				Source:              h.domain,
				CanRunInTransaction: true,
			}, stmt)
		}
	}

	for _, h := range d.heldTableDependents {
		table := h.table
		tableName := getTableNameWithSchema(table.Schema, table.Name, targetSchema)
		drop := func(diffType DiffType, operation DiffOperation, name string, source DiffSource, stmt string) {
			collector.collect(&diffContext{
				Type:                diffType,
				Operation:           operation,
				Path:                fmt.Sprintf("%s.%s.%s", table.Schema, table.Name, name),
				Source:              source,
				CanRunInTransaction: true,
			}, stmt)
		}
		for _, trigger := range h.droppedTriggers {
			drop(DiffTypeTableTrigger, DiffOperationRecreate, trigger.Name, trigger, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON %s;", ir.QuoteIdentifier(trigger.Name), tableName))
		}
		for _, policy := range h.droppedPolicies {
			drop(DiffTypeTablePolicy, DiffOperationRecreate, policy.Name, policy, fmt.Sprintf("DROP POLICY IF EXISTS %s ON %s;", ir.QuoteIdentifier(policy.Name), tableName))
		}
		for _, constraint := range h.droppedConstraints {
			drop(DiffTypeTableConstraint, DiffOperationRecreate, constraint.Name, constraint, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", tableName, ir.QuoteIdentifier(constraint.Name)))
		}
		for _, index := range h.droppedIndexes {
			drop(DiffTypeTableIndex, DiffOperationRecreate, index.Name, index, fmt.Sprintf("DROP INDEX IF EXISTS %s;", qualifyEntityName(index.Schema, index.Name, targetSchema)))
		}
		for _, col := range h.droppedDefaults {
			drop(DiffTypeTableColumn, DiffOperationAlter, col.Name, col, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", onlyTable(table, tableName), ir.QuoteIdentifier(col.Name)))
		}
	}
}

// generateRestoreHeldFunctionDependentsSQL creates the desired version of
// every held object once the functions it calls have been created again,
// using the same statements as any other added default, constraint, index,
// policy or trigger. (#601)
//
// The objects replacing a version dropped for the recreation are created in
// the same transaction, so the table is never without them: a rebuilt index
// is created without CONCURRENTLY (see the plan's online rewrites), and a
// CHECK constraint on an existing table is added NOT VALID, which enforces it
// for new rows at once. The steps needing transactions of their own follow
// only after all of them are back (generateCompleteHeldFunctionDependentsSQL).
func (d *ddlDiff) generateRestoreHeldFunctionDependentsSQL(targetSchema string, collector *diffCollector) {
	defer markHeldDependents(collector, len(collector.diffs))
	for _, h := range d.heldDomainDependents {
		domainName := qualifyEntityName(h.domain.Schema, h.domain.Name, targetSchema)
		var statements []string
		if h.setDefault {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s SET DEFAULT %s;", domainName, h.domain.Default))
		}
		for _, c := range h.constraints {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s ADD CONSTRAINT %s %s;", domainName, ir.QuoteIdentifier(c.Name), c.Definition))
		}
		for _, stmt := range statements {
			collector.collect(&diffContext{
				Type:                DiffTypeDomain,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", h.domain.Schema, h.domain.Name),
				Source:              h.domain,
				CanRunInTransaction: true,
			}, stmt)
		}
	}

	for _, h := range d.heldTableDependents {
		tableName := getTableNameWithSchema(h.table.Schema, h.table.Name, targetSchema)
		for _, col := range h.defaults {
			collector.collect(&diffContext{
				Type:                DiffTypeTableColumn,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s.%s", h.table.Schema, h.table.Name, col.Name),
				Source:              col,
				CanRunInTransaction: true,
			}, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", onlyTable(h.table, tableName), ir.QuoteIdentifier(col.Name), *col.DefaultValue))
		}
		restore := &tableDiff{
			Table:         h.table,
			AddedPolicies: h.policies,
			AddedTriggers: h.triggers,
		}
		for _, constraint := range h.constraints {
			if constraint.Type == ir.ConstraintTypeCheck && h.existing {
				notValid := *constraint
				notValid.IsValid = false
				constraint = &notValid
			}
			restore.AddedConstraints = append(restore.AddedConstraints, constraint)
		}
		for _, index := range h.indexes {
			if !h.newIndex(index) {
				restore.AddedIndexes = append(restore.AddedIndexes, index)
			}
		}
		restore.generateAlterTableStatements(targetSchema, collector, nil, nil, nil, nil)
	}
}

// generateCompleteHeldFunctionDependentsSQL emits the steps that follow the
// restore in transactions of their own: VALIDATE CONSTRAINT for the CHECK
// constraints re-added NOT VALID on existing tables, and the indexes that did
// not exist before, created CONCURRENTLY like any added index. (#601)
func (d *ddlDiff) generateCompleteHeldFunctionDependentsSQL(targetSchema string, collector *diffCollector) {
	for _, h := range d.heldTableDependents {
		for _, constraint := range h.constraints {
			if constraint.Type != ir.ConstraintTypeCheck || !h.existing || !constraint.IsValid {
				continue
			}
			collector.collect(&diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", constraint.Schema, constraint.Table, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}, fmt.Sprintf("ALTER TABLE %s VALIDATE CONSTRAINT %s;",
				getTableNameWithSchema(constraint.Schema, constraint.Table, targetSchema), ir.QuoteIdentifier(constraint.Name)))
		}
	}
	for _, h := range d.heldTableDependents {
		added := &tableDiff{Table: h.table}
		for _, index := range h.indexes {
			if h.newIndex(index) {
				added.AddedIndexes = append(added.AddedIndexes, index)
			}
		}
		if len(added.AddedIndexes) > 0 {
			added.generateAlterTableStatements(targetSchema, collector, nil, nil, nil, nil)
		}
	}
}

// newIndex reports whether a held index is added to an existing table rather
// than replacing a version dropped for the recreation.
func (h *heldTableDependents) newIndex(index *ir.Index) bool {
	if !h.existing {
		return false
	}
	for _, dropped := range h.droppedIndexes {
		if dropped.Name == index.Name {
			return false
		}
	}
	return true
}

// markHeldDependents flags the diffs collected since from as held
// dependents (see Diff.HeldDependent).
func markHeldDependents(collector *diffCollector, from int) {
	for i := from; i < len(collector.diffs); i++ {
		collector.diffs[i].HeldDependent = true
	}
}

// ValidateFunctionRecreations returns an error when the migration from oldIR
// to newIR drops and creates again a function (a return type or parameter
// change, #326) that an object calls which pgschema does not carry through
// that cycle (#601):
//
//   - a generated column: it would have to be dropped and added again around
//     the function (a table rewrite that also drops what depends on the
//     column), which is not done here;
//   - a column the migration adds to an existing table, or re-creates with
//     DROP + ADD COLUMN (#591), whose DEFAULT - its own, or else that of its
//     domain type (following domains over domains) - calls the function: its
//     existing rows are filled when the column is added, before the function
//     exists again (by the old function, or with NULL where the default is
//     held back);
//   - an existing column whose type changes and whose new DEFAULT calls the
//     function: ALTER COLUMN TYPE runs before the function, and the default
//     cannot stay held across it.
//
// targetMajorVersion decides which generation-clause changes re-create a
// column, as in GenerateMigrationForTarget. Matching is by function name, as
// for the objects that are handled.
func ValidateFunctionRecreations(oldIR, newIR *ir.IR, targetMajorVersion int) error {
	oldFunctions := functionsBySignature(oldIR)
	newFunctions := functionsBySignature(newIR)
	var recreated []*ir.Function
	for _, key := range sortedKeys(newFunctions) {
		if old, ok := oldFunctions[key]; ok && functionRequiresRecreate(old, newFunctions[key]) {
			recreated = append(recreated, newFunctions[key])
		}
	}
	if len(recreated) == 0 {
		return nil
	}
	lookup := buildRoutineLookup(recreated, nil)
	// callee names the recreated function(s) an expression calls.
	callee := func(expr, schema string) string {
		var names []string
		for _, fn := range recreated {
			if referencesNewFunction(expr, schema, buildRoutineLookup([]*ir.Function{fn}, nil)) {
				names = append(names, fmt.Sprintf("%s.%s(%s)", fn.Schema, fn.Name, fn.GetArguments()))
			}
		}
		return strings.Join(names, ", ")
	}

	oldTables := tablesByName(oldIR)
	newTables := tablesByName(newIR)
	domains := make(map[string]*ir.Type)
	for _, dbSchema := range newIR.Schemas {
		for _, typ := range dbSchema.Types {
			if typ.Kind == ir.TypeKindDomain {
				domains[strings.ToLower(typ.Schema+"."+typ.Name)] = typ
			}
		}
	}
	var problems []string
	for _, key := range sortedKeys(newTables) {
		table := newTables[key]
		if table.IsExternal {
			continue
		}
		oldTable := oldTables[key]
		oldColumns := make(map[string]*ir.Column)
		if oldTable != nil {
			for _, col := range oldTable.Columns {
				oldColumns[col.Name] = col
			}
		}
		for _, col := range table.Columns {
			name := fmt.Sprintf("%s.%s.%s", table.Schema, table.Name, col.Name)
			if col.IsGenerated && col.GeneratedExpr != nil && referencesNewFunction(*col.GeneratedExpr, table.Schema, lookup) {
				if oldTable == nil {
					problems = append(problems, fmt.Sprintf("generated column %s of new table %s.%s calls %s, and the table would be created while the old function still exists; create the table in a separate step after the function change",
						name, table.Schema, table.Name, callee(*col.GeneratedExpr, table.Schema)))
				} else {
					problems = append(problems, fmt.Sprintf("generated column %s calls %s; keeping it would require dropping and re-adding the column (a table rewrite, and its dependents with it), which is not done here; change or drop the column in a separate step first",
						name, callee(*col.GeneratedExpr, table.Schema)))
				}
				continue
			}
			// The rows a column added to an existing table (or re-created)
			// already has are filled from its default, or else from its
			// domain's, when ADD COLUMN runs: before the function exists again.
			if oldTable == nil {
				continue
			}
			oldCol := oldColumns[col.Name]
			var how string
			switch {
			case oldCol == nil:
				how = "is added"
			case generatedColumnNeedsRecreate(oldCol, col, targetMajorVersion):
				how = "is re-created (DROP + ADD COLUMN)"
			default:
				// The default is held around the function, but ALTER COLUMN
				// TYPE runs before it and would need the current default
				// re-set on the new type (or the column left without one).
				if col.DefaultValue != nil && referencesNewFunction(*col.DefaultValue, table.Schema, lookup) && columnTypeChanges(oldCol, col, table.Schema) {
					problems = append(problems, fmt.Sprintf("column %s changes its type or collation and gets a DEFAULT that calls %s; change the column type or collation in a separate step, before or after the function change",
						name, callee(*col.DefaultValue, table.Schema)))
				}
				continue
			}
			defaultValue, source := "", "a DEFAULT that"
			if col.DefaultValue != nil {
				defaultValue = *col.DefaultValue
			} else if domain := domainWithDefault(col.DataType, table.Schema, domains); domain != nil {
				defaultValue = domain.Default
				source = fmt.Sprintf("type %s.%s, whose DEFAULT", domain.Schema, domain.Name)
			}
			if referencesNewFunction(defaultValue, table.Schema, lookup) {
				problems = append(problems, fmt.Sprintf("column %s %s with %s calls %s, so its existing rows would be filled before the function is created again; add or change the column in a separate step after the function change",
					name, how, source, callee(defaultValue, table.Schema)))
			}
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("cannot drop and re-create functions whose return type or parameters change, because these objects call them:\n  - %s",
		strings.Join(problems, "\n  - "))
}

// columnTypeChanges reports whether the column diff alters the column's type
// or collation (see ColumnDiff.generateColumnSQL).
func columnTypeChanges(old, new *ir.Column, schema string) bool {
	return stripSchemaPrefix(comparableColumnType(old), schema) != stripSchemaPrefix(comparableColumnType(new), schema) ||
		stripSchemaPrefix(old.Collation, schema) != stripSchemaPrefix(new.Collation, schema)
}

// domainWithDefault returns the domain whose default a column of the given
// type gets: the type itself if it is a domain with a default, or else the
// nearest domain with one it is (transitively) based on.
func domainWithDefault(typeName, schema string, domains map[string]*ir.Type) *ir.Type {
	for depth := 0; depth < 16 && typeName != ""; depth++ {
		if strings.HasSuffix(strings.TrimSpace(typeName), "]") {
			return nil // an array of a domain does not get the domain's default
		}
		name := strings.ToLower(strings.ReplaceAll(extractBaseTypeName(typeName), `"`, ""))
		domain := domains[name]
		if domain == nil && !strings.Contains(name, ".") {
			domain = domains[strings.ToLower(schema)+"."+name]
		}
		if domain == nil {
			return nil
		}
		if domain.Default != "" {
			return domain
		}
		typeName, schema = domain.BaseType, domain.Schema
	}
	return nil
}

func functionsBySignature(schemaIR *ir.IR) map[string]*ir.Function {
	functions := make(map[string]*ir.Function)
	for _, dbSchema := range schemaIR.Schemas {
		for signature, fn := range dbSchema.Functions {
			functions[fn.Schema+"."+signature] = fn
		}
	}
	return functions
}

func tablesByName(schemaIR *ir.IR) map[string]*ir.Table {
	tables := make(map[string]*ir.Table)
	for _, dbSchema := range schemaIR.Schemas {
		for _, table := range dbSchema.Tables {
			tables[table.Schema+"."+table.Name] = table
		}
	}
	return tables
}
