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
//     merge to (inheritedColumnDefinition). Two cases keep the column in
//     the CREATE TABLE instead: parents that disagree on the default, which
//     PostgreSQL refuses unless the child settles it, and a child's own
//     generation expression, which has no other place. An override on a
//     column a parent only adds in this migration runs after the parent's
//     ADD COLUMN, in the modify phase.
//   - On a modified child, a column it gets from a parent it keeps is
//     neither added nor dropped, and only its DEFAULT, NOT NULL and
//     generation expression are diffed, rendered the same way; the parent's
//     own diff carries everything else. Parents are ordered before children
//     in both the create and the modify phases, so a child-local object
//     that needs a column the parent is adding comes after the parent's
//     ADD COLUMN.
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
	var defaults []string
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
			if column.IsGenerated && !merged.IsGenerated {
				merged.IsGenerated = true
				merged.GeneratedKind = column.GeneratedKind
				merged.GeneratedExpr = column.GeneratedExpr
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
	return merged, conflict
}

// inheritedColumnDeclaredInline reports whether an inherited column must
// still be declared in the child's CREATE TABLE: when its parents disagree
// on the default, PostgreSQL refuses the table unless the child settles it,
// and a child's own generation expression can only be given there. The
// declaration merges with the parents' definition.
func inheritedColumnDeclaredInline(table *ir.Table, column *ir.Column, allTables map[string]*ir.Table) bool {
	if !column.IsInherited {
		return false
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

// childColumnDrop is a DROP COLUMN a child needs because its parent's drop
// of the column does not reach it (see planInheritanceEffects).
type childColumnDrop struct {
	table  *ir.Table
	column string
}

// inheritedColumnOverrideStatements returns the statements that set the
// DEFAULT and NOT NULL a child sets locally on one column it inherits, or
// nothing when the column matches what the parents pass on. tableName is the
// child's rendered name.
func inheritedColumnOverrideStatements(table *ir.Table, column *ir.Column, tableName string, allTables map[string]*ir.Table) []string {
	if !column.IsInherited || inheritedColumnDeclaredInline(table, column, allTables) {
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

// planInheritanceEffects records, on each modified table's diff, what its
// column changes do to its inheritance children, since PostgreSQL recurses
// them:
//
//   - A dropped column is dropped from every child as well. A child whose
//     desired state keeps the column (it declares it, as the parent no longer
//     does) needs the parent to drop with ONLY, which leaves the column on
//     every child as a local column; the children that do not keep it then
//     drop it themselves. Without ONLY, only a child that also declared the
//     column (IsMergedInherited) keeps it, and such a child drops it itself
//     when the desired state has no column.
//   - An added column arrives on every descendant with the parent's
//     definition, and a changed DEFAULT or NOT NULL overwrites what a
//     descendant set on it. The descendants' overrides on that column are
//     re-applied after the parent's statements.
//
// oldTables and newTables key the tables of each state by schema.name.
func planInheritanceEffects(modifiedTables []*tableDiff, oldTables, newTables map[string]*ir.Table, droppedTables []*ir.Table, targetSchema string) {
	oldChildren := inheritanceChildren(oldTables)
	newChildren := inheritanceChildren(newTables)
	dropped := make(map[string]bool, len(droppedTables))
	for _, table := range droppedTables {
		dropped[table.Schema+"."+table.Name] = true
	}
	for _, td := range modifiedTables {
		key := td.Table.Schema + "." + td.Table.Name

		for _, column := range td.DroppedColumns {
			if td.RecreatedColumns[column.Name] {
				continue
			}
			var keepers, losers, mergedLosers []*ir.Table
			for _, child := range oldChildren[key] {
				childKey := child.Schema + "." + child.Name
				if dropped[childKey] {
					continue
				}
				desired := newTables[childKey]
				if desired != nil && findColumn(desired, column.Name) != nil {
					keepers = append(keepers, child)
				} else if oldColumn := findColumn(child, column.Name); oldColumn != nil && oldColumn.IsMergedInherited {
					mergedLosers = append(mergedLosers, child)
				} else {
					losers = append(losers, child)
				}
			}
			if len(keepers) > 0 {
				if td.DropColumnOnly == nil {
					td.DropColumnOnly = map[string]bool{}
				}
				td.DropColumnOnly[column.Name] = true
				for _, child := range losers {
					td.ChildColumnDrops = append(td.ChildColumnDrops, childColumnDrop{table: child, column: column.Name})
				}
			}
			for _, child := range mergedLosers {
				td.ChildColumnDrops = append(td.ChildColumnDrops, childColumnDrop{table: child, column: column.Name})
			}
		}

		var affected []string
		for _, column := range td.AddedColumns {
			affected = append(affected, column.Name)
		}
		for _, cd := range td.ModifiedColumns {
			if cd.Old.IsNullable != cd.New.IsNullable || columnDefault(cd.Old) != columnDefault(cd.New) {
				affected = append(affected, cd.New.Name)
			}
		}
		if len(affected) == 0 {
			continue
		}
		for _, descendant := range inheritanceDescendants(newChildren, key) {
			tableName := getTableNameWithSchema(descendant.Schema, descendant.Name, targetSchema)
			for _, name := range affected {
				if column := findColumn(descendant, name); column != nil {
					td.ChildOverrides = append(td.ChildOverrides, inheritedColumnOverrideStatements(descendant, column, tableName, newTables)...)
				}
			}
		}
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
	var supplied []*ir.Constraint
	for _, parent := range td.Table.Inherits {
		if containsTableRef(td.OldInherits, parent) {
			continue
		}
		supplied = append(supplied, inheritableCheckConstraints(tables, tables[parent.Schema+"."+parent.Name])...)
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
