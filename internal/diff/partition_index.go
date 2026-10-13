package diff

import (
	"fmt"
	"sort"

	"github.com/pgplex/pgschema/ir"
)

// availablePartitionIndexes includes indexes created by retained constraints.
// Those indexes remain outside standalone index creation and comparison.
func availablePartitionIndexes(tables []*ir.Table) map[ir.IndexRef]bool {
	available := make(map[ir.IndexRef]bool)
	for _, table := range tables {
		for _, index := range table.Indexes {
			available[ir.IndexRef{Schema: index.Schema, Name: index.Name}] = true
		}
		for _, constraint := range table.Constraints {
			if constraint.BackingIndex != nil {
				available[*constraint.BackingIndex] = true
			}
		}
	}
	return available
}

// validatePartitionIndexDependencies rejects filtered index families before any
// dump statements are generated. Migration generation still honors filtering.
func validatePartitionIndexDependencies(schema *ir.IR) error {
	indexes := make(map[ir.IndexRef]*ir.Index)
	var keys []ir.IndexRef
	var tables []*ir.Table
	tablesByName := make(map[string]*ir.Table)
	constraintIndexes := make(map[ir.IndexRef]*ir.Table)
	for _, namespace := range schema.Schemas {
		for _, table := range namespace.Tables {
			tables = append(tables, table)
			tablesByName[table.Schema+"."+table.Name] = table
			for _, constraint := range table.Constraints {
				if constraint.BackingIndex != nil {
					constraintIndexes[*constraint.BackingIndex] = table
				}
			}
			for _, index := range table.Indexes {
				key := ir.IndexRef{Schema: index.Schema, Name: index.Name}
				indexes[key] = index
				keys = append(keys, key)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Schema != keys[j].Schema {
			return keys[i].Schema < keys[j].Schema
		}
		return keys[i].Name < keys[j].Name
	})
	available := availablePartitionIndexes(tables)
	for _, key := range keys {
		for _, child := range indexes[key].AttachedPartitions {
			if !available[child] {
				return fmt.Errorf("cannot dump index %s: attached index %s is excluded; adjust .pgschemaignore to include the child index, its table, and any owning constraint",
					ir.QuoteIdentifier(key.Schema)+"."+ir.QuoteIdentifier(key.Name),
					ir.QuoteIdentifier(child.Schema)+"."+ir.QuoteIdentifier(child.Name))
			}
			if table := constraintIndexes[child]; table != nil {
				if err := validateConstraintIndexPartitions(key, child, table, tablesByName); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Constraint generation recursively creates an index on every table partition.
// An attached constraint-owned index therefore requires all of those tables,
// including descendants whose inherited constraints are not separate IR objects.
func validateConstraintIndexPartitions(parent, child ir.IndexRef, table *ir.Table, tables map[string]*ir.Table) error {
	pending := []*ir.Table{table}
	seen := make(map[string]bool)
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		key := current.Schema + "." + current.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		for _, partition := range current.AllPartitionChildren {
			next := tables[partition.Schema+"."+partition.Name]
			if next == nil || next.IsExternal {
				return fmt.Errorf("cannot dump index %s: attached constraint-backed index %s requires excluded partition table %s; adjust .pgschemaignore to include the partition table",
					ir.QuoteIdentifier(parent.Schema)+"."+ir.QuoteIdentifier(parent.Name),
					ir.QuoteIdentifier(child.Schema)+"."+ir.QuoteIdentifier(child.Name),
					ir.QuoteIdentifier(partition.Schema)+"."+ir.QuoteIdentifier(partition.Name))
			}
			pending = append(pending, next)
		}
	}
	return nil
}

// partitionIndexCreationPlan belongs to ddlDiff. It schedules explicit index
// creation only after the tables and named leaf indexes are available.
// Migrations select complete, entirely new table hierarchies. Existing or
// filtered hierarchies retain recursive creation for compatibility.
// Dump selects every managed partitioned index after its strict validation.
type partitionIndexCreationPlan struct {
	indexes      map[ir.IndexRef]*ir.Index
	children     map[ir.IndexRef]map[ir.IndexRef]bool
	uniqueTables map[string]bool
}

func planPartitionIndexCreation(addedTables []*ir.Table, desiredTables map[string]*ir.Table, dump bool) *partitionIndexCreationPlan {
	plan := &partitionIndexCreationPlan{
		indexes:      make(map[ir.IndexRef]*ir.Index),
		children:     make(map[ir.IndexRef]map[ir.IndexRef]bool),
		uniqueTables: make(map[string]bool),
	}
	added := make(map[string]*ir.Table)
	available := availablePartitionIndexes(addedTables)
	indexes := make(map[ir.IndexRef]*ir.Index)
	for _, table := range addedTables {
		added[table.Schema+"."+table.Name] = table
		for _, index := range table.Indexes {
			if index.Type != ir.IndexTypePrimary {
				indexes[ir.IndexRef{Schema: index.Schema, Name: index.Name}] = index
			}
		}
	}

	roots := make(map[string]string)
	blocked := make(map[string]bool)
	if !dump {
		for key, table := range desiredTables {
			root := partitionHierarchyRoot(table, desiredTables)
			roots[key] = root
			if root == "" {
				continue
			}
			if added[key] == nil || table.IsExternal {
				blocked[root] = true
			}
			// A missing table can change index validity even when it has no
			// attached index. Check table membership as well as index edges.
			for _, child := range table.AllPartitionChildren {
				if added[child.Schema+"."+child.Name] == nil {
					blocked[root] = true
				}
			}
			for _, index := range table.Indexes {
				for _, child := range index.AttachedPartitions {
					if !available[child] {
						blocked[root] = true
					}
				}
				if index.ParentIndex != "" && !available[ir.IndexRef{Schema: index.ParentIndexSchema, Name: index.ParentIndex}] {
					blocked[root] = true
				}
			}
		}
	}

	for _, table := range addedTables {
		key := table.Schema + "." + table.Name
		if !dump && (roots[key] == "" || blocked[roots[key]]) {
			continue
		}
		for _, index := range table.Indexes {
			if index.IsPartitioned && index.Type != ir.IndexTypePrimary {
				plan.indexes[ir.IndexRef{Schema: index.Schema, Name: index.Name}] = index
				if index.Type == ir.IndexTypeUnique {
					plan.uniqueTables[key] = true
				}
			}
		}
	}
	for parent, index := range plan.indexes {
		plan.children[parent] = make(map[ir.IndexRef]bool)
		for _, child := range index.AttachedPartitions {
			plan.children[parent][child] = true
		}
	}
	// ParentIndex also preserves standalone edges in IR without transient
	// inspection metadata, such as an IR restored from JSON.
	for _, index := range indexes {
		parent := ir.IndexRef{Schema: index.ParentIndexSchema, Name: index.ParentIndex}
		if children := plan.children[parent]; children != nil {
			children[ir.IndexRef{Schema: index.Schema, Name: index.Name}] = true
		}
	}
	return plan
}

// partitionHierarchyRoot returns an empty key for incomplete or cyclic ancestry.
func partitionHierarchyRoot(table *ir.Table, tables map[string]*ir.Table) string {
	seen := make(map[string]bool)
	for table != nil && !table.IsExternal {
		key := table.Schema + "." + table.Name
		if seen[key] {
			return ""
		}
		seen[key] = true
		if table.PartitionOf == "" {
			return key
		}
		parentSchema := table.PartitionOfSchema
		if parentSchema == "" {
			parentSchema = table.Schema
		}
		table = tables[parentSchema+"."+table.PartitionOf]
	}
	return ""
}

func (p *partitionIndexCreationPlan) contains(index *ir.Index) bool {
	return p != nil && p.indexes[ir.IndexRef{Schema: index.Schema, Name: index.Name}] != nil
}

func (p *partitionIndexCreationPlan) defersForeignKey(table *ir.Table, constraint *ir.Constraint) bool {
	if p == nil || constraint == nil || constraint.Type != ir.ConstraintTypeForeignKey {
		return false
	}
	schema := constraint.ReferencedSchema
	if schema == "" {
		schema = table.Schema
	}
	return p.uniqueTables[schema+"."+constraint.ReferencedTable]
}

// generateSQL creates partitioned children before their parents. ON ONLY avoids
// implicitly creating or choosing leaf indexes; only recorded children attach.
func (p *partitionIndexCreationPlan) generateSQL(targetSchema string, collector *diffCollector) {
	if p == nil {
		return
	}
	keys := make([]ir.IndexRef, 0, len(p.indexes))
	for key := range p.indexes {
		keys = append(keys, key)
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
		attached := make([]ir.IndexRef, 0, len(p.children[key]))
		for child := range p.children[key] {
			attached = append(attached, child)
		}
		sort.Slice(attached, func(i, j int) bool {
			if attached[i].Schema != attached[j].Schema {
				return attached[i].Schema < attached[j].Schema
			}
			return attached[i].Name < attached[j].Name
		})
		for _, child := range attached {
			if p.indexes[child] != nil {
				create(child)
			}
		}
		index := p.indexes[key]
		generateCreateIndexSQL(index, targetSchema, collector, DiffTypeTableIndex, DiffTypeTableIndexComment, true)
		for _, child := range attached {
			sql := fmt.Sprintf("ALTER INDEX %s ATTACH PARTITION %s;",
				qualifyEntityNameMode(index.Schema, index.Name, targetSchema, collector.qualifySchema),
				qualifyEntityNameMode(child.Schema, child.Name, targetSchema, collector.qualifySchema))
			collector.collect(&diffContext{
				Type: DiffTypeTableIndex, Operation: DiffOperationAlter,
				Path:   fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
				Source: index, CanRunInTransaction: true,
			}, sql)
		}
	}
	for _, key := range keys {
		create(key)
	}
}
