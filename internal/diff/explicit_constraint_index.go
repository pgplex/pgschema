package diff

import (
	"fmt"
	"sort"

	"github.com/pgplex/pgschema/ir"
)

type explicitConstraintIndex struct {
	constraint *ir.Constraint
	index      ir.IndexRef
	parent     ir.IndexRef
	definition string
	comment    string
}

// planExplicitConstraints selects whole, new table hierarchies within one
// creation batch. A function/domain boundary retains recursive creation and
// strict restoration, because consumers may need the parent's key earlier.
func (p *partitionIndexCreationPlan) planExplicitConstraints(tables map[string]*ir.Table, batches ...[]*ir.Table) {
	if p == nil {
		return
	}
	p.constraints = make(map[ir.IndexRef]*explicitConstraintIndex)
	p.constraintTables = make(map[string]bool)
	batchOf := make(map[string]int)
	for b, batch := range batches {
		for _, t := range batch {
			batchOf[t.Schema+"."+t.Name] = b + 1
		}
	}
	families := make(map[string][]*ir.Table)
	blocked := make(map[string]bool)
	for key, t := range tables {
		root := partitionHierarchyRoot(t, tables)
		if root == "" {
			continue
		}
		families[root] = append(families[root], t)
		if batchOf[key] == 0 || batchOf[key] != batchOf[root] {
			blocked[root] = true
		}
		for _, child := range t.AllPartitionChildren {
			if batchOf[child.Schema+"."+child.Name] == 0 {
				blocked[root] = true
			}
		}
	}
	for root, family := range families {
		if blocked[root] || len(family) < 2 {
			continue
		}
		candidates := make(map[ir.IndexRef]*explicitConstraintIndex)
		valid := true
		for _, t := range family {
			key := t.Schema + "." + t.Name
			// Ignored origins and incomplete inspection metadata stay on the old path.
			if len(p.inherited[key]) != len(t.InheritedConstraintIndexes) {
				valid = false
			}
			for _, c := range t.Constraints {
				if c.Type != ir.ConstraintTypePrimaryKey && c.Type != ir.ConstraintTypeUnique {
					continue
				}
				if c.CatalogDefinition == "" || c.BackingIndex == nil || c.BackingIndex.Name != c.Name || c.IsTemporal {
					valid = false
					continue
				}
				candidates[*c.BackingIndex] = &explicitConstraintIndex{constraint: c, index: *c.BackingIndex, definition: c.CatalogDefinition, comment: c.CatalogComment}
			}
			for _, c := range p.inherited[key] {
				if c.Definition == "" || c.Index.Name != c.ConstraintName {
					valid = false
					continue
				}
				constraint := &ir.Constraint{Schema: t.Schema, Table: t.Name, Name: c.ConstraintName, Type: c.ConstraintType}
				candidates[c.Index] = &explicitConstraintIndex{constraint: constraint, index: c.Index, parent: c.ParentIndex, definition: c.Definition, comment: c.Comment}
			}
		}
		for _, c := range candidates {
			if c.parent.Name != "" && candidates[c.parent] == nil {
				valid = false
			}
			t := tables[c.constraint.Schema+"."+c.constraint.Table]
			children := make(map[ir.TableDependency]bool)
			for _, child := range t.AllPartitionChildren {
				children[child] = true
			}
			for _, child := range family {
				parentSchema := child.PartitionOfSchema
				if parentSchema == "" {
					parentSchema = child.Schema
				}
				if child.PartitionOf == t.Name && parentSchema == t.Schema {
					children[ir.TableDependency{Schema: child.Schema, Name: child.Name}] = true
				}
			}
			for child := range children {
				found := false
				for _, cc := range candidates {
					if cc.parent == c.index && cc.constraint.Schema == child.Schema && cc.constraint.Table == child.Name {
						found = true
						break
					}
				}
				if !found {
					valid = false
				}
			}
		}
		if !valid || len(candidates) == 0 {
			continue
		}
		for key, c := range candidates {
			p.constraints[key] = c
			tableKey := c.constraint.Schema + "." + c.constraint.Table
			p.constraintTables[tableKey] = true
			p.uniqueTables[tableKey] = true
		}
	}
}

func (p *partitionIndexCreationPlan) explicitConstraint(c *ir.Constraint) bool {
	if p == nil || c == nil {
		return false
	}
	planned := p.constraints[ir.IndexRef{Schema: c.Schema, Name: c.Name}]
	return planned != nil && planned.constraint.Table == c.Table && planned.constraint.Type == c.Type
}

// Finish each batch before functions or policies can consume its constraints.
// Create children first and attach exact catalog edges without index matching.
func (p *partitionIndexCreationPlan) generateExplicitConstraintsSQL(batch []*ir.Table, targetSchema string, collector *diffCollector) {
	if p == nil {
		return
	}
	selected := make(map[string]bool)
	for _, t := range batch {
		selected[t.Schema+"."+t.Name] = true
	}
	var keys []ir.IndexRef
	for key, c := range p.constraints {
		if selected[c.constraint.Schema+"."+c.constraint.Table] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Schema != keys[j].Schema {
			return keys[i].Schema < keys[j].Schema
		}
		return keys[i].Name < keys[j].Name
	})
	created := make(map[ir.IndexRef]bool)
	var create func(ir.IndexRef)
	create = func(key ir.IndexRef) {
		if created[key] {
			return
		}
		created[key] = true
		c := p.constraints[key]
		for _, child := range keys {
			if p.constraints[child].parent == key {
				create(child)
			}
		}
		constraint := c.constraint
		tableName := qualifyEntityNameMode(constraint.Schema, constraint.Table, targetSchema, collector.qualifySchema)
		context := &diffContext{Type: DiffTypeTableConstraint, Operation: DiffOperationCreate, Path: constraint.Schema + "." + constraint.Table + "." + constraint.Name, Source: constraint, CanRunInTransaction: true}
		collector.collect(context, fmt.Sprintf("ALTER TABLE ONLY %s ADD CONSTRAINT %s %s;", tableName, ir.QuoteIdentifier(constraint.Name), c.definition))
		indexType := ir.IndexTypeUnique
		if constraint.Type == ir.ConstraintTypePrimaryKey {
			indexType = ir.IndexTypePrimary
		}
		index := &ir.Index{Schema: key.Schema, Table: constraint.Table, Name: key.Name, Type: indexType, IsPartitioned: true}
		for _, child := range keys {
			if p.constraints[child].parent != key {
				continue
			}
			attachment := &diffContext{Type: DiffTypeTableIndex, Operation: DiffOperationAttach, Path: index.Schema + "." + index.Table + "." + index.Name, Source: index, CanRunInTransaction: true}
			collector.collect(attachment, fmt.Sprintf("ALTER INDEX %s ATTACH PARTITION %s;", qualifyEntityNameMode(key.Schema, key.Name, targetSchema, collector.qualifySchema), qualifyEntityNameMode(child.Schema, child.Name, targetSchema, collector.qualifySchema)))
		}
		if c.comment != "" {
			comment := *context
			comment.Type = DiffTypeTableComment
			comment.Path = constraint.Schema + "." + constraint.Table
			collector.collect(&comment, fmt.Sprintf("COMMENT ON CONSTRAINT %s ON %s IS %s;", ir.QuoteIdentifier(constraint.Name), tableName, quoteString(c.comment)))
		}
	}
	for _, key := range keys {
		create(key)
	}
}
