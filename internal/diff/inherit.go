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
//     it has any. The column is compared against the first parent that
//     defines it, as PostgreSQL merges them.
//   - On a modified child whose parents are unchanged, inherited columns are
//     neither added nor dropped, and only their DEFAULT and NOT NULL are
//     diffed, rendered the same way; the parent's own diff carries
//     everything else. Parents are ordered before children in both the
//     create and the modify phases, so a child-local object that needs a
//     column the parent is adding comes after the parent's ADD COLUMN.
//   - A changed parent list is applied as ALTER TABLE child NO INHERIT old
//     in the drop phase, before DROP TABLE could take the child along with
//     its parent, and ALTER TABLE child INHERIT new at the end of the table
//     modifications, once the child has every column and CHECK constraint
//     the new parent requires. While the parent list changes, columns are
//     compared without regard to IsInherited: NO INHERIT turns inherited
//     columns into local ones, and INHERIT merges local columns with the
//     parent's.
//
// A column that is inherited on one side and merged (declared in the child
// as well) on the other is not a difference: PostgreSQL has no DDL for that
// transition, and the column behaves the same either way until a parent
// drops it.

// inheritsEqual reports whether two parent lists are the same, in order.
func inheritsEqual(a, b []ir.TableRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
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

// inheritanceParentColumn returns the definition an inherited column takes
// from the table's parents: the first parent, in INHERITS order, that defines
// a column of that name. Nil when no parent is among allTables, e.g. a parent
// in an unmanaged schema.
func inheritanceParentColumn(table *ir.Table, name string, allTables map[string]*ir.Table) *ir.Column {
	for _, parent := range table.Inherits {
		parentTable := allTables[parent.Schema+"."+parent.Name]
		if parentTable == nil {
			continue
		}
		for _, column := range parentTable.Columns {
			if column.Name == name {
				return column
			}
		}
	}
	return nil
}

// inheritedColumnOverrideSQL returns the statements that follow a child's
// CREATE TABLE to set the DEFAULT and NOT NULL the child sets locally on
// columns it inherits. They are part of the table's create step.
func inheritedColumnOverrideSQL(table *ir.Table, targetSchema string, qualifySchema bool, allTables map[string]*ir.Table) []string {
	if len(table.Inherits) == 0 {
		return nil
	}
	var statements []string
	tableName := ir.QualifyEntityNameWithQuotesMode(table.Schema, table.Name, targetSchema, qualifySchema)
	for _, column := range table.Columns {
		if !column.IsInherited {
			continue
		}
		parentColumn := inheritanceParentColumn(table, column.Name, allTables)
		if parentColumn == nil {
			continue
		}
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
	}
	return statements
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
		for _, constraint := range td.MissingParentChecks {
			sql := fmt.Sprintf("ALTER TABLE ONLY %s ADD %s;", tableName, generateConstraintSQL(constraint, targetSchema, false))
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
	for _, parent := range oldTable.Inherits {
		if containsTableRef(newTable.Inherits, parent) {
			continue
		}
		for _, source := range inheritableCheckConstraints(tables, tables[parent.Schema+"."+parent.Name]) {
			if table.Constraints[source.Name] != nil {
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
	for _, constraint := range td.DroppedConstraints {
		if constraint.Type == ir.ConstraintTypeCheck && parentSuppliesCheck(supplied, constraint) {
			continue
		}
		kept = append(kept, constraint)
	}
	td.DroppedConstraints = kept
	for _, source := range supplied {
		if oldTable.Constraints[source.Name] != nil || td.Table.Constraints[source.Name] != nil {
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
