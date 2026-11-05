package diff

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// Table inheritance (CREATE TABLE ... INHERITS).
//
// A child table gets its parents' columns and CHECK constraints from the
// parents, and PostgreSQL keeps them in step: ADD/DROP COLUMN, ALTER COLUMN
// TYPE, SET/DROP NOT NULL and ADD/DROP CONSTRAINT ... CHECK on a parent
// recurse to its children, while the same statements on the child for an
// inherited column or constraint are rejected. The IR reflects this: the
// inspector marks purely inherited columns (Column.IsInherited) and leaves the
// inherited CHECK copies out, so the diff treats them as follows.
//
//   - CREATE TABLE prints only local columns and ends with INHERITS (...).
//     A local DEFAULT or NOT NULL on an inherited column, the only things a
//     child may change about it, follow the CREATE TABLE: the default as
//     ALTER TABLE ONLY child ALTER COLUMN ... SET DEFAULT (pg_dump's
//     rendering; without ONLY it would also overwrite the defaults of the
//     child's own children), the NOT NULL without ONLY, since NOT NULL
//     recurses to the child's children in any case and ONLY is refused once
//     it has any. The column is compared against the definition the parents
//     merge to (inheritedColumnDefinition). The column stays in the CREATE
//     TABLE instead when the child declared it itself (it then survives the
//     parent's DROP COLUMN, which the declaration preserves), when the
//     parents disagree on the default or generation expression, which
//     PostgreSQL refuses unless the child settles it, and for a child's own
//     generation expression, which has no other place. An override on a
//     column a parent only adds in this migration runs after the parent's
//     ADD COLUMN, in the modify phase.
//   - On a modified child, a column it gets from a parent it keeps is
//     neither added nor dropped, and only its DEFAULT, NOT NULL, generation
//     expression and comment are diffed, rendered the same way; the
//     parent's own diff carries everything else, and plans what its
//     recursive statements do to the descendants (planInheritanceEffects):
//     a column a descendant keeps is dropped with ONLY down to it, a column
//     or CHECK a descendant also declared survives the parent's drop and is
//     dropped by the descendant, and what descendants set on a column the
//     parent adds or changes is re-applied afterwards. Parents are ordered
//     before children in both the create and the modify phases, so a
//     child-local object that needs a column the parent is adding comes
//     after the parent's ADD COLUMN.
//   - A changed parent list is applied as ALTER TABLE child NO INHERIT old
//     in the drop phase, before DROP TABLE could take the child along with
//     its parent, and ALTER TABLE child INHERIT new at the end of the table
//     modifications, once the child has every column and CHECK constraint
//     the new parent requires; a required CHECK the child lacks, or holds
//     under the same name with another expression, is added right before
//     the INHERIT. Columns from a parent the child leaves or joins are
//     compared like local ones: NO INHERIT turns them into local columns,
//     and INHERIT merges local columns with the parent's. CHECK constraints
//     follow the same split: those the child shared only with a parent it
//     leaves are local after NO INHERIT and reconciled against the desired
//     table; those a retained parent still supplies stay inherited.
//
// A column that is inherited on one side and merged (declared in the child
// as well) on the other is not a difference: PostgreSQL has no DDL for that
// transition, and the column behaves the same either way until a parent
// drops it.

// inheritsEqual reports whether two parent lists name the same parents. The
// order of parents (inhseqno) is not managed: INHERIT and NO INHERIT can only
// append and remove, so a reordered list is treated as unchanged.
func inheritsEqual(a, b []ir.TableRef) bool {
	if len(a) != len(b) {
		return false
	}
	for _, ref := range a {
		if !containsTableRef(b, ref) {
			return false
		}
	}
	return true
}

// diffInherits records a changed parent list on the diff.
func diffInherits(diff *tableDiff, oldTable, newTable *ir.Table) {
	if inheritsEqual(oldTable.Inherits, newTable.Inherits) {
		return
	}
	diff.InheritsChanged = true
	diff.OldInherits = oldTable.Inherits
}

// inheritsClauseSQL returns the INHERITS (...) clause of a child table, or ""
// for a table without parents.
func inheritsClauseSQL(table *ir.Table, targetSchema string, qualifySchema bool) string {
	if len(table.Inherits) == 0 {
		return ""
	}
	parents := make([]string, 0, len(table.Inherits))
	for _, parent := range table.Inherits {
		parents = append(parents, ir.QualifyEntityNameWithQuotesMode(parent.Schema, parent.Name, targetSchema, qualifySchema))
	}
	return " INHERITS (" + strings.Join(parents, ", ") + ")"
}

// inheritedColumnDefinition returns the definition an inherited column takes
// from the table's parents, merged the way PostgreSQL merges them: NOT NULL
// if any parent has it, the default the parents agree on, and the generation
// expression of the first generated parent. conflict is set when the parents
// carry different defaults, which the child must resolve by declaring the
// column itself. Nil when no parent among allTables defines the column, e.g.
// when the parents live in an unmanaged schema.
func inheritedColumnDefinition(table *ir.Table, name string, allTables map[string]*ir.Table) (merged *ir.Column, conflict bool) {
	var defaults, expressions []string
	for _, parent := range table.Inherits {
		parentTable := allTables[parent.Schema+"."+parent.Name]
		if parentTable == nil {
			continue
		}
		for _, column := range parentTable.Columns {
			if column.Name != name {
				continue
			}
			if merged == nil {
				merged = &ir.Column{Name: name, IsNullable: true}
			}
			if !column.IsNullable {
				merged.IsNullable = false
			}
			if d := columnDefault(column); d != "" && !slices.Contains(defaults, d) {
				defaults = append(defaults, d)
			}
			if column.IsGenerated {
				if !merged.IsGenerated {
					merged.IsGenerated = true
					merged.GeneratedKind = column.GeneratedKind
					merged.GeneratedExpr = column.GeneratedExpr
				}
				if e := generatedExpr(column); !slices.Contains(expressions, e) {
					expressions = append(expressions, e)
				}
			}
			break
		}
	}
	if merged == nil {
		return nil, false
	}
	switch len(defaults) {
	case 0:
	case 1:
		merged.DefaultValue = &defaults[0]
	default:
		conflict = true
	}
	// Parents generating the column differently also need the child to settle it
	if len(expressions) > 1 {
		conflict = true
	}
	return merged, conflict
}

// inlineDeclarationNeedsDefaultNull reports whether an inherited column the
// child declares inline must carry DEFAULT NULL: the parents disagree on the
// default and the child has none (it dropped the one it settled with), and a
// declaration without a default would not resolve the conflict. The DROP
// DEFAULT follows the CREATE TABLE.
func inlineDeclarationNeedsDefaultNull(table *ir.Table, column *ir.Column, allTables map[string]*ir.Table) bool {
	// A generated column has no default; its declared expression settles
	// a conflict between the parents' expressions on its own
	if !column.IsInherited || column.IsGenerated || columnDefault(column) != "" {
		return false
	}
	merged, conflict := inheritedColumnDefinition(table, column.Name, allTables)
	return merged != nil && conflict
}

// inheritedColumnDeclaredInline reports whether an inherited column is
// declared in the child's CREATE TABLE as well: when the child declared it
// itself (IsMergedInherited, kept so that the column goes on surviving the
// parent's DROP COLUMN, as pg_dump keeps it), when its parents disagree on
// the default or the generation expression, which PostgreSQL refuses unless
// the child settles it, and when the child has a generation expression of
// its own, which can only be given there. The declaration merges with the
// parents' definition.
func inheritedColumnDeclaredInline(table *ir.Table, column *ir.Column, allTables map[string]*ir.Table) bool {
	if !column.IsInherited {
		return false
	}
	if column.IsMergedInherited {
		return true
	}
	merged, conflict := inheritedColumnDefinition(table, column.Name, allTables)
	if merged == nil {
		return false
	}
	return conflict || generatedExpr(column) != generatedExpr(merged)
}

// ancestorAddsColumn reports whether a parent of the table, or a parent of a
// parent, is adding the column in this migration (added keys schema.table to
// the column names its ALTER TABLE adds). The child's override of such a
// column has to wait for that ADD COLUMN.
func ancestorAddsColumn(table *ir.Table, name string, allTables map[string]*ir.Table, added map[string]map[string]bool) bool {
	visited := map[string]bool{}
	var walk func(t *ir.Table) bool
	walk = func(t *ir.Table) bool {
		for _, parent := range t.Inherits {
			key := parent.Schema + "." + parent.Name
			if visited[key] {
				continue
			}
			visited[key] = true
			if added[key][name] {
				return true
			}
			if parentTable := allTables[key]; parentTable != nil && walk(parentTable) {
				return true
			}
		}
		return false
	}
	return walk(table)
}

// addedParentColumns maps schema.table of every modified table to the names
// of the columns its ALTER TABLE adds, for ancestorAddsColumn.
func addedParentColumns(modifiedTables []*tableDiff) map[string]map[string]bool {
	added := map[string]map[string]bool{}
	for _, td := range modifiedTables {
		if len(td.AddedColumns) == 0 {
			continue
		}
		columns := make(map[string]bool, len(td.AddedColumns))
		for _, column := range td.AddedColumns {
			columns[column.Name] = true
		}
		added[td.Table.Schema+"."+td.Table.Name] = columns
	}
	return added
}

// childColumnDrop is a DROP COLUMN an inheritance descendant needs because
// its parent's drop of the column does not reach it, or must not (only).
// They run after all table modifications, since a column shared with
// several parents can only go once every parent has dropped it. See
// planInheritanceEffects.
type childColumnDrop struct {
	table  *ir.Table
	column string
	only   bool
	// depth below the parent whose drop planned it; the aggregated drops
	// run parents first
	depth int
}

// inheritanceChildEffects are the statements descendants need after all
// table modifications (see planInheritanceEffects).
type inheritanceChildEffects struct {
	columnDrops []childColumnDrop
	checkDrops  []childConstraintDrop
}

// childConstraintDrop is a DROP CONSTRAINT a child needs right after its
// parent's, for a CHECK copy the child also declared (MergedInheritedChecks)
// and therefore keeps.
type childConstraintDrop struct {
	table      *ir.Table
	constraint string
}

// childOverride re-applies, on a descendant, the DEFAULT and NOT NULL it
// sets on a column its parent adds or changes; the diff's Old side is the
// state the parent's recursive ALTER leaves behind.
type childOverride struct {
	table *ir.Table
	diff  *ColumnDiff
}

// inheritedColumnOverrideStatements returns the statements that set the
// DEFAULT and NOT NULL a child sets locally on one column it inherits, or
// nothing when the column matches what the parents pass on. tableName is the
// child's rendered name.
func inheritedColumnOverrideStatements(table *ir.Table, column *ir.Column, tableName string, allTables map[string]*ir.Table) []string {
	if !column.IsInherited {
		return nil
	}
	if inheritedColumnDeclaredInline(table, column, allTables) {
		if inlineDeclarationNeedsDefaultNull(table, column, allTables) {
			return []string{fmt.Sprintf("ALTER TABLE ONLY %s ALTER COLUMN %s DROP DEFAULT;", tableName, ir.QuoteIdentifier(column.Name))}
		}
		return nil
	}
	parentColumn, _ := inheritedColumnDefinition(table, column.Name, allTables)
	if parentColumn == nil {
		return nil
	}
	var statements []string
	childDefault, parentDefault := columnDefault(column), columnDefault(parentColumn)
	switch {
	case childDefault != "" && childDefault != parentDefault:
		statements = append(statements, fmt.Sprintf("ALTER TABLE ONLY %s ALTER COLUMN %s SET DEFAULT %s;", tableName, ir.QuoteIdentifier(column.Name), childDefault))
	case childDefault == "" && parentDefault != "":
		statements = append(statements, fmt.Sprintf("ALTER TABLE ONLY %s ALTER COLUMN %s DROP DEFAULT;", tableName, ir.QuoteIdentifier(column.Name)))
	}
	if !column.IsNullable && parentColumn.IsNullable {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tableName, ir.QuoteIdentifier(column.Name)))
	}
	return statements
}

// inheritedColumnOverrideSQL returns the statements that follow a child's
// CREATE TABLE to set the DEFAULT and NOT NULL the child sets locally on
// columns it inherits. They are part of the table's create step. A column a
// parent is only adding in this migration (parentAdded keys schema.table to
// the columns its ALTER TABLE adds) is left out: the parent's diff re-applies
// the child's overrides after its ADD COLUMN (see planInheritanceEffects).
func inheritedColumnOverrideSQL(table *ir.Table, targetSchema string, qualifySchema bool, allTables map[string]*ir.Table, parentAdded map[string]map[string]bool) []string {
	if len(table.Inherits) == 0 {
		return nil
	}
	var statements []string
	tableName := ir.QualifyEntityNameWithQuotesMode(table.Schema, table.Name, targetSchema, qualifySchema)
	for _, column := range table.Columns {
		if ancestorAddsColumn(table, column.Name, allTables, parentAdded) {
			continue
		}
		statements = append(statements, inheritedColumnOverrideStatements(table, column, tableName, allTables)...)
	}
	return statements
}

// inheritanceChildren indexes the tables of one state by the schema.name of
// each parent they inherit from.
func inheritanceChildren(tables map[string]*ir.Table) map[string][]*ir.Table {
	children := map[string][]*ir.Table{}
	for _, key := range sortedKeys(tables) {
		table := tables[key]
		for _, parent := range table.Inherits {
			parentKey := parent.Schema + "." + parent.Name
			children[parentKey] = append(children[parentKey], table)
		}
	}
	return children
}

// inheritanceDescendants lists every table that inherits from the given one,
// directly or through other tables, parents before their children.
func inheritanceDescendants(children map[string][]*ir.Table, key string) []*ir.Table {
	var descendants []*ir.Table
	visited := map[string]bool{}
	queue := []string{key}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			childKey := child.Schema + "." + child.Name
			if visited[childKey] {
				continue
			}
			visited[childKey] = true
			descendants = append(descendants, child)
			queue = append(queue, childKey)
		}
	}
	return descendants
}

// inheritanceEffectsPlanner carries the two states while planInheritanceEffects
// walks the hierarchies.
type inheritanceEffectsPlanner struct {
	oldTables, newTables     map[string]*ir.Table
	oldChildren, newChildren map[string][]*ir.Table
	dropped                  map[string]bool
	diffs                    map[string]*tableDiff
	targetSchema             string
}

// planInheritanceEffects records, on each modified table's diff, what its
// column and CHECK changes do to its inheritance descendants, since
// PostgreSQL recurses them, and returns the DROP COLUMN statements the
// descendants need after all table modifications:
//
//   - A dropped column is dropped from every descendant as well. A child
//     whose desired state keeps the column (it declares it, as the parent no
//     longer does) needs the parent to drop with ONLY, which leaves the
//     column on every child as a local column; a child that does not keep it
//     but has a descendant that does drops it with ONLY in turn, and the
//     others drop it themselves. Without ONLY, only a descendant that also
//     declared the column (IsMergedInherited) keeps it, and drops it itself.
//     A child leaving the parent in this migration is left to its own diff.
//   - A dropped or changed CHECK constraint is dropped from a child that
//     also declared it (MergedInheritedChecks) only if the child's desired
//     state does not declare it with the same expression.
//   - An added column arrives on every descendant with the parent's
//     definition, and a changed DEFAULT or NOT NULL overwrites what every
//     descendant set on it. The descendants' desired DEFAULT and NOT NULL
//     are re-applied against that state after the parent's statements, on
//     the descendants attached before this migration and the new ones; a
//     table that only joins the parent now brings the column itself.
//
// oldTables and newTables key the tables of each state by schema.name.
func planInheritanceEffects(modifiedTables []*tableDiff, oldTables, newTables map[string]*ir.Table, droppedTables []*ir.Table, targetSchema string) inheritanceChildEffects {
	p := &inheritanceEffectsPlanner{
		oldTables:    oldTables,
		newTables:    newTables,
		oldChildren:  inheritanceChildren(oldTables),
		newChildren:  inheritanceChildren(newTables),
		dropped:      make(map[string]bool, len(droppedTables)),
		diffs:        make(map[string]*tableDiff, len(modifiedTables)),
		targetSchema: targetSchema,
	}
	for _, table := range droppedTables {
		p.dropped[table.Schema+"."+table.Name] = true
	}
	for _, td := range modifiedTables {
		p.diffs[td.Table.Schema+"."+td.Table.Name] = td
	}
	var effects inheritanceChildEffects
	for _, td := range modifiedTables {
		key := td.Table.Schema + "." + td.Table.Name
		effects.columnDrops = append(effects.columnDrops, p.planColumnDrops(td, key)...)
		effects.checkDrops = append(effects.checkDrops, p.planCheckDrops(td, key)...)
		p.planOverrides(td, key)
	}
	return effects
}

// newChildren lists the desired-state children of parentKey created in this
// migration. They are created before the table modifications, so they
// inherit the parent's old columns like the children attached before.
func (p *inheritanceEffectsPlanner) newlyCreatedChildren(parentKey string) []*ir.Table {
	var created []*ir.Table
	for _, child := range p.newChildren[parentKey] {
		if _, existed := p.oldTables[child.Schema+"."+child.Name]; !existed {
			created = append(created, child)
		}
	}
	return created
}

// attachedChildren lists the old-state children of parentKey that still
// inherit from it in the desired state; the others are dropped or leave the
// parent, and their own diff handles their columns.
func (p *inheritanceEffectsPlanner) attachedChildren(parentKey string) []*ir.Table {
	parent := p.oldTables[parentKey]
	if parent == nil {
		return nil
	}
	ref := ir.TableRef{Schema: parent.Schema, Name: parent.Name}
	var attached []*ir.Table
	for _, child := range p.oldChildren[parentKey] {
		childKey := child.Schema + "." + child.Name
		if p.dropped[childKey] {
			continue
		}
		desired := p.newTables[childKey]
		if desired == nil || !containsTableRef(desired.Inherits, ref) {
			continue
		}
		attached = append(attached, child)
	}
	return attached
}

func (p *inheritanceEffectsPlanner) planColumnDrops(td *tableDiff, key string) []childColumnDrop {
	var drops []childColumnDrop
	for _, column := range td.DroppedColumns {
		if td.RecreatedColumns[column.Name] {
			continue
		}
		keeper, childDrops := p.planColumnDrop(key, column.Name, 1)
		if keeper {
			if td.DropColumnOnly == nil {
				td.DropColumnOnly = map[string]bool{}
			}
			td.DropColumnOnly[column.Name] = true
		}
		drops = append(drops, childDrops...)
	}
	return drops
}

// planColumnDrop plans the drop of column below parentKey. keeper reports
// whether a descendant keeps the column, so the parent has to drop with
// ONLY; drops are the statements the descendants then need.
func (p *inheritanceEffectsPlanner) planColumnDrop(parentKey, column string, depth int) (keeper bool, drops []childColumnDrop) {
	type pending struct {
		child  *ir.Table
		merged bool
		keeper bool
		drops  []childColumnDrop
	}
	var pendings []pending
	consider := func(child *ir.Table, merged bool) {
		childKey := child.Schema + "." + child.Name
		if findColumn(p.newTables[childKey], column) != nil {
			keeper = true
			return
		}
		childKeeper, childDrops := p.planColumnDrop(childKey, column, depth+1)
		if childKeeper {
			keeper = true
		}
		pendings = append(pendings, pending{child: child, merged: merged, keeper: childKeeper, drops: childDrops})
	}
	for _, child := range p.attachedChildren(parentKey) {
		if oldColumn := findColumn(child, column); oldColumn != nil {
			consider(child, oldColumn.IsMergedInherited)
		}
	}
	// Children created in this migration got the column at creation too
	for _, child := range p.newlyCreatedChildren(parentKey) {
		consider(child, false)
	}
	for _, c := range pendings {
		switch {
		case c.keeper:
			drops = append(drops, childColumnDrop{table: c.child, column: column, only: true, depth: depth})
		case keeper || c.merged:
			drops = append(drops, childColumnDrop{table: c.child, column: column, depth: depth})
		}
		drops = append(drops, c.drops...)
	}
	return keeper, drops
}

// planCheckDrops plans what a parent's dropped or changed CHECK does to the
// copies its descendants also declared. A copy another desired parent still
// supplies stays inherited and is left alone. A copy left behind by a check
// the parent drops outright is dropped after all table modifications, once
// every parent has let go of it (returned); one left behind by a check the
// parent changes is dropped right after the parent's DROP, before the
// parent's ADD recurses (recorded on the diff).
func (p *inheritanceEffectsPlanner) planCheckDrops(td *tableDiff, key string) []childConstraintDrop {
	var deferred []childConstraintDrop
	plan := func(check *ir.Constraint, inline bool) {
		for _, child := range inheritanceDescendants(p.oldChildren, key) {
			childKey := child.Schema + "." + child.Name
			if !child.MergedInheritedChecks[check.Name] || p.dropped[childKey] {
				continue
			}
			desired := p.newTables[childKey]
			if desired == nil {
				continue
			}
			// Still inherited from another parent the child keeps
			stillSupplied := false
			for _, parent := range desired.Inherits {
				parentKey := parent.Schema + "." + parent.Name
				if parentKey != key && parentSuppliesCheck(inheritableCheckConstraints(p.newTables, p.newTables[parentKey]), check) {
					stillSupplied = true
					break
				}
			}
			if stillSupplied {
				continue
			}
			// Parent gone, child still declares it with the same expression:
			// the local copy that survives is the desired constraint
			if kept := desired.Constraints[check.Name]; kept != nil && kept.CheckClause == check.CheckClause {
				if childDiff := p.diffs[childKey]; childDiff != nil {
					childDiff.AddedConstraints = slices.DeleteFunc(childDiff.AddedConstraints, func(c *ir.Constraint) bool { return c.Name == check.Name })
				}
				continue
			}
			drop := childConstraintDrop{table: child, constraint: check.Name}
			if inline {
				td.ChildConstraintDrops = append(td.ChildConstraintDrops, drop)
			} else {
				deferred = append(deferred, drop)
			}
		}
	}
	for _, constraint := range td.DroppedConstraints {
		if constraint.Type == ir.ConstraintTypeCheck {
			plan(constraint, false)
		}
	}
	for _, cd := range td.ModifiedConstraints {
		if cd.Old.Type == ir.ConstraintTypeCheck {
			plan(cd.Old, true)
		}
	}
	return deferred
}

// planOverrides plans the DEFAULT, NOT NULL and generation expression the
// descendants need back after this table's column statements. A statement
// that recurses (ADD COLUMN, SET/DROP NOT NULL, and SET DEFAULT / SET
// EXPRESSION on a table's own column) overwrites every descendant; one with
// ONLY (SET DEFAULT / SET EXPRESSION on an inherited column) reaches none,
// but a descendant created in this migration, before the statement, was
// created with the old value and needs the new one.
func (p *inheritanceEffectsPlanner) planOverrides(td *tableDiff, key string) {
	type effect struct {
		after                               *ir.Column
		defaultRecurses, expressionRecurses bool
	}
	effects := map[string]effect{}
	var names []string
	for _, column := range td.AddedColumns {
		effects[column.Name] = effect{after: column, defaultRecurses: true, expressionRecurses: true}
		names = append(names, column.Name)
	}
	for _, cd := range td.ModifiedColumns {
		defaultChanged := columnDefault(cd.Old) != columnDefault(cd.New)
		expressionChanged := generatedExpr(cd.Old) != generatedExpr(cd.New)
		if cd.Old.IsNullable == cd.New.IsNullable && !defaultChanged && !expressionChanged {
			continue
		}
		after := *cd.New
		if cd.Inherited {
			// ONLY: the descendants keep what they had
			after.DefaultValue = cd.Old.DefaultValue
			after.GeneratedExpr = cd.Old.GeneratedExpr
		}
		effects[cd.New.Name] = effect{after: &after, defaultRecurses: !cd.Inherited, expressionRecurses: !cd.Inherited}
		names = append(names, cd.New.Name)
	}
	if len(names) == 0 {
		return
	}
	attachedBefore := map[string]bool{}
	for _, d := range inheritanceDescendants(p.oldChildren, key) {
		attachedBefore[d.Schema+"."+d.Name] = true
	}
	for _, descendant := range inheritanceDescendants(p.newChildren, key) {
		descendantKey := descendant.Schema + "." + descendant.Name
		_, existed := p.oldTables[descendantKey]
		if existed && !attachedBefore[descendantKey] {
			continue // joins the parent at the end of the modify phase, with its own columns
		}
		for _, name := range names {
			desired := findColumn(descendant, name)
			if desired == nil {
				continue
			}
			e := effects[name]
			// An existing descendant's own diff covers what the parent's ONLY
			// statement leaves untouched
			left := *desired
			left.IsNullable = e.after.IsNullable
			left.InvalidNotNullConstraint = ""
			if e.defaultRecurses || !existed {
				left.DefaultValue = e.after.DefaultValue
			}
			if e.expressionRecurses || !existed {
				left.GeneratedExpr = e.after.GeneratedExpr
			}
			if inheritedColumnChanged(&left, desired) {
				td.ChildOverrides = append(td.ChildOverrides, childOverride{
					table: descendant,
					diff:  &ColumnDiff{Old: &left, New: desired, Inherited: true},
				})
			}
		}
	}
}

// generateInheritanceChildDropsSQL emits the DROP COLUMN statements the
// descendants of modified parents need once every parent has dropped the
// column (see planInheritanceEffects). The same drop planned from several
// parents is emitted once, with ONLY if any of them requires it.
func generateInheritanceChildDropsSQL(effects inheritanceChildEffects, targetSchema string, collector *diffCollector) {
	type dropKey struct{ table, column string }
	only := map[dropKey]bool{}
	depth := map[dropKey]int{}
	var order []dropKey
	tables := map[dropKey]*ir.Table{}
	for _, drop := range effects.columnDrops {
		k := dropKey{drop.table.Schema + "." + drop.table.Name, drop.column}
		if _, seen := only[k]; !seen {
			order = append(order, k)
			tables[k] = drop.table
		}
		only[k] = only[k] || drop.only
		depth[k] = max(depth[k], drop.depth)
	}
	// Parents before children: a child's copy can only go once the tables
	// above it have let go of the column
	slices.SortStableFunc(order, func(a, b dropKey) int { return depth[a] - depth[b] })
	for _, k := range order {
		table := tables[k]
		tableName := getTableNameWithSchema(table.Schema, table.Name, targetSchema)
		if only[k] {
			tableName = "ONLY " + tableName
		}
		collector.collect(&diffContext{
			Type:                DiffTypeTableColumn,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s.%s", table.Schema, table.Name, k.column),
			Source:              table,
			CanRunInTransaction: true,
		}, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", tableName, ir.QuoteIdentifier(k.column)))
	}

	seen := map[string]bool{}
	for _, drop := range effects.checkDrops {
		k := drop.table.Schema + "." + drop.table.Name + "." + drop.constraint
		if seen[k] {
			continue
		}
		seen[k] = true
		tableName := getTableNameWithSchema(drop.table.Schema, drop.table.Name, targetSchema)
		collector.collect(&diffContext{
			Type:                DiffTypeTableConstraint,
			Operation:           DiffOperationDrop,
			Path:                k,
			Source:              drop.table,
			CanRunInTransaction: true,
		}, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", tableName, ir.QuoteIdentifier(drop.constraint)))
	}
}

// findColumn returns the table's column of that name, or nil.
func findColumn(table *ir.Table, name string) *ir.Column {
	for _, column := range table.Columns {
		if column.Name == name {
			return column
		}
	}
	return nil
}

// parentDefinesColumn reports whether one of the given parents, or one of
// their own parents, defines the column. tables keys every table of the same
// state by schema.name.
func parentDefinesColumn(tables map[string]*ir.Table, parents []ir.TableRef, name string) bool {
	visited := map[string]bool{}
	var walk func(refs []ir.TableRef) bool
	walk = func(refs []ir.TableRef) bool {
		for _, ref := range refs {
			key := ref.Schema + "." + ref.Name
			if visited[key] {
				continue
			}
			visited[key] = true
			parent := tables[key]
			if parent == nil {
				continue
			}
			for _, column := range parent.Columns {
				if column.Name == name {
					return true
				}
			}
			if walk(parent.Inherits) {
				return true
			}
		}
		return false
	}
	return walk(parents)
}

// retainedParents returns the parents a child keeps across a parent list
// change.
func retainedParents(oldTable, newTable *ir.Table) []ir.TableRef {
	var retained []ir.TableRef
	for _, parent := range newTable.Inherits {
		if containsTableRef(oldTable.Inherits, parent) {
			retained = append(retained, parent)
		}
	}
	return retained
}

// columnDefault returns the column's default expression, or "" when it has
// none.
func columnDefault(column *ir.Column) string {
	if column.DefaultValue == nil {
		return ""
	}
	return *column.DefaultValue
}

// generateNoInheritSQL emits NO INHERIT for every modified table that leaves
// a parent. It belongs in the drop phase, before tables are dropped; see the
// note at the top of this file.
func generateNoInheritSQL(diffs []*tableDiff, targetSchema string, collector *diffCollector) {
	for _, td := range diffs {
		if !td.InheritsChanged {
			continue
		}
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		for _, parent := range td.OldInherits {
			if containsTableRef(td.Table.Inherits, parent) {
				continue
			}
			parentName := getTableNameWithSchema(parent.Schema, parent.Name, targetSchema)
			sql := fmt.Sprintf("ALTER TABLE %s NO INHERIT %s;", tableName, parentName)
			collector.collect(td.inheritContext(DiffOperationNoInherit), sql)
		}
	}
}

// generateInheritSQL emits INHERIT for every modified table that joins a
// parent. It belongs at the end of the table modifications; see the note at
// the top of this file.
func generateInheritSQL(diffs []*tableDiff, targetSchema string, collector *diffCollector) {
	for _, td := range diffs {
		if !td.InheritsChanged {
			continue
		}
		tableName := getTableNameWithSchema(td.Table.Schema, td.Table.Name, targetSchema)
		// The CHECK constraints the new parents require and the child lacks;
		// INHERIT merges them with the parents' (see reconcileConstraintsForInherit)
		// (without ONLY: the child's own children need it too, and ONLY is
		// refused for an inheritable CHECK on a table that has children)
		for _, constraint := range td.MissingParentChecks {
			sql := fmt.Sprintf("ALTER TABLE %s ADD %s;", tableName, generateConstraintSQL(constraint, targetSchema, false))
			collector.collect(&diffContext{
				Type:                DiffTypeTableConstraint,
				Operation:           DiffOperationCreate,
				Path:                fmt.Sprintf("%s.%s.%s", td.Table.Schema, td.Table.Name, constraint.Name),
				Source:              constraint,
				CanRunInTransaction: true,
			}, sql)
		}
		for _, parent := range td.Table.Inherits {
			if containsTableRef(td.OldInherits, parent) {
				continue
			}
			parentName := getTableNameWithSchema(parent.Schema, parent.Name, targetSchema)
			sql := fmt.Sprintf("ALTER TABLE %s INHERIT %s;", tableName, parentName)
			collector.collect(td.inheritContext(DiffOperationInherit), sql)
		}
	}
}

func containsTableRef(refs []ir.TableRef, ref ir.TableRef) bool {
	return slices.Contains(refs, ref)
}

// inheritableCheckConstraints lists the CHECK constraints a table passes on
// to its inheritance children: its own, except NO INHERIT ones, plus those it
// inherits itself. tables keys every table of the same state by schema.name;
// parents outside it contribute nothing.
func inheritableCheckConstraints(tables map[string]*ir.Table, table *ir.Table) []*ir.Constraint {
	var constraints []*ir.Constraint
	visited := map[*ir.Table]bool{}
	var walk func(t *ir.Table)
	walk = func(t *ir.Table) {
		if t == nil || visited[t] {
			return
		}
		visited[t] = true
		for _, name := range sortedKeys(t.Constraints) {
			constraint := t.Constraints[name]
			if constraint.Type == ir.ConstraintTypeCheck && !constraint.NoInherit {
				constraints = append(constraints, constraint)
			}
		}
		for _, parent := range t.Inherits {
			walk(tables[parent.Schema+"."+parent.Name])
		}
	}
	walk(table)
	return constraints
}

// withConstraintsRetainedByNoInherit returns a copy of the child oldTable
// whose constraints include the CHECK constraints it shares with the parents
// it is leaving. The inspector leaves those out of a child's IR, but once NO
// INHERIT has run they are ordinary local constraints: a desired table that
// declares the same constraint must not add it again, and one that does not
// must drop it. tables keys every table of the old state by schema.name.
func withConstraintsRetainedByNoInherit(oldTable, newTable *ir.Table, tables map[string]*ir.Table) *ir.Table {
	table := *oldTable
	table.Constraints = make(map[string]*ir.Constraint, len(oldTable.Constraints))
	for name, constraint := range oldTable.Constraints {
		table.Constraints[name] = constraint
	}
	// A check the child still gets from a parent it keeps stays inherited
	var stillSupplied []*ir.Constraint
	for _, parent := range retainedParents(oldTable, newTable) {
		stillSupplied = append(stillSupplied, inheritableCheckConstraints(tables, tables[parent.Schema+"."+parent.Name])...)
	}
	for _, parent := range oldTable.Inherits {
		if containsTableRef(newTable.Inherits, parent) {
			continue
		}
		for _, source := range inheritableCheckConstraints(tables, tables[parent.Schema+"."+parent.Name]) {
			if table.Constraints[source.Name] != nil || parentSuppliesCheck(stillSupplied, source) {
				continue
			}
			retained := *source
			retained.Schema = oldTable.Schema
			retained.Table = oldTable.Name
			retained.Comment = ""
			table.Constraints[source.Name] = &retained
		}
	}
	return &table
}

// reconcileConstraintsForInherit prepares the diff of a table joining new
// parents. ALTER TABLE ... INHERIT requires the child to already carry each
// CHECK constraint of the parent, under the same name, and merges them: a
// matching constraint the child has is kept rather than dropped (the desired
// child does not list it, since the inspector leaves shared constraints out),
// and one it lacks is added just before the INHERIT (see generateInheritSQL).
// oldTable is the child's current state; tables keys every table of the
// desired state by schema.name.
func (td *tableDiff) reconcileConstraintsForInherit(oldTable *ir.Table, tables map[string]*ir.Table) {
	var supplied, retainedSupplied []*ir.Constraint
	for _, parent := range td.Table.Inherits {
		parentChecks := inheritableCheckConstraints(tables, tables[parent.Schema+"."+parent.Name])
		if containsTableRef(td.OldInherits, parent) {
			retainedSupplied = append(retainedSupplied, parentChecks...)
		} else {
			supplied = append(supplied, parentChecks...)
		}
	}
	if len(supplied) == 0 {
		return
	}
	kept := td.DroppedConstraints[:0]
	dropped := map[string]bool{}
	for _, constraint := range td.DroppedConstraints {
		if constraint.Type == ir.ConstraintTypeCheck && parentSuppliesCheck(supplied, constraint) {
			continue
		}
		kept = append(kept, constraint)
		dropped[constraint.Name] = true
	}
	td.DroppedConstraints = kept
	for _, source := range supplied {
		// Present only if the child has it and keeps it: an old constraint of
		// the same name with another expression is dropped above and must
		// be replaced, or the INHERIT would find it missing.
		if (oldTable.Constraints[source.Name] != nil && !dropped[source.Name]) || td.Table.Constraints[source.Name] != nil {
			continue
		}
		// Already on the child through a parent it keeps
		if parentSuppliesCheck(retainedSupplied, source) {
			continue
		}
		if slices.ContainsFunc(td.MissingParentChecks, func(c *ir.Constraint) bool { return c.Name == source.Name }) {
			continue
		}
		missing := *source
		missing.Schema = td.Table.Schema
		missing.Table = td.Table.Name
		missing.Comment = ""
		td.MissingParentChecks = append(td.MissingParentChecks, &missing)
	}
}

// parentSuppliesCheck reports whether a parent passes on a CHECK constraint
// with the same name and expression, which INHERIT merges with the child's.
func parentSuppliesCheck(supplied []*ir.Constraint, constraint *ir.Constraint) bool {
	for _, candidate := range supplied {
		if candidate.Name == constraint.Name && candidate.CheckClause == constraint.CheckClause {
			return true
		}
	}
	return false
}

// inheritContext is the diff context of an INHERIT / NO INHERIT statement.
func (td *tableDiff) inheritContext(operation DiffOperation) *diffContext {
	return &diffContext{
		Type:                DiffTypeTableInherit,
		Operation:           operation,
		Path:                fmt.Sprintf("%s.%s", td.Table.Schema, td.Table.Name),
		Source:              td.Table,
		CanRunInTransaction: true,
	}
}
