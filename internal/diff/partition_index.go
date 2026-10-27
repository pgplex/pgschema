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
	// constraints holds, per table key, the PRIMARY KEY and UNIQUE constraints
	// added after every partition exists instead of inline with CREATE TABLE.
	// PARTITION OF copies an inline constraint onto the new partition under a
	// generated name, which can take the name of a standalone index created
	// later (issue #653). Explicit ALTER TABLE ONLY ... ADD CONSTRAINT
	// statements reproduce the catalog names instead.
	constraints map[string]map[string]bool
	// partitions lists the added partitions of each table key, in name order.
	partitions map[string][]*ir.Table
	tables     map[string]*ir.Table
	// roots maps each table of a hierarchy with postponed constraints to the
	// hierarchy's root key, so the whole hierarchy is created in one batch.
	roots map[string]string
	// created and emitted track creation progress so each constraint family
	// is added as soon as its last partition exists, ahead of SQL functions
	// whose bodies depend on the key.
	created map[string]bool
	emitted map[string]map[string]bool
}

func planPartitionIndexCreation(addedTables []*ir.Table, desiredTables map[string]*ir.Table, dump bool) *partitionIndexCreationPlan {
	plan := &partitionIndexCreationPlan{
		indexes:      make(map[ir.IndexRef]*ir.Index),
		children:     make(map[ir.IndexRef]map[ir.IndexRef]bool),
		uniqueTables: make(map[string]bool),
		constraints:  make(map[string]map[string]bool),
		partitions:   make(map[string][]*ir.Table),
		roots:        make(map[string]string),
		created:      make(map[string]bool),
		emitted:      make(map[string]map[string]bool),
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
	plan.tables = added
	for _, table := range addedTables {
		if table.PartitionOf == "" {
			continue
		}
		parentSchema := table.PartitionOfSchema
		if parentSchema == "" {
			parentSchema = table.Schema
		}
		parentKey := parentSchema + "." + table.PartitionOf
		plan.partitions[parentKey] = append(plan.partitions[parentKey], table)
	}
	for _, partitions := range plan.partitions {
		sort.Slice(partitions, func(i, j int) bool {
			if partitions[i].Schema != partitions[j].Schema {
				return partitions[i].Schema < partitions[j].Schema
			}
			return partitions[i].Name < partitions[j].Name
		})
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
		// A constraint is postponed only when every partition copy's catalog
		// name is known; otherwise inline creation remains the fallback.
		if len(plan.partitions[key]) == 0 {
			continue
		}
		for _, name := range sortedKeys(table.Constraints) {
			constraint := table.Constraints[name]
			if constraint.Type != ir.ConstraintTypePrimaryKey && constraint.Type != ir.ConstraintTypeUnique {
				continue
			}
			if !plan.inheritedNamesKnown(table, name) {
				continue
			}
			if plan.constraints[key] == nil {
				plan.constraints[key] = make(map[string]bool)
			}
			plan.constraints[key][name] = true
			// Foreign keys referencing the table or any of its partitions wait
			// for the postponed constraints.
			plan.markUniqueTables(table)
		}
	}
	for key := range plan.constraints {
		root := key
		for {
			table := added[root]
			if table == nil || table.PartitionOf == "" {
				break
			}
			parentSchema := table.PartitionOfSchema
			if parentSchema == "" {
				parentSchema = table.Schema
			}
			parentKey := parentSchema + "." + table.PartitionOf
			if added[parentKey] == nil {
				break
			}
			root = parentKey
		}
		plan.markHierarchy(added[root], root)
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

// inheritedNamesKnown reports whether every partition below table, at any
// depth, records the catalog name of its copy of the named constraint.
func (p *partitionIndexCreationPlan) inheritedNamesKnown(table *ir.Table, constraintName string) bool {
	for _, child := range p.partitions[table.Schema+"."+table.Name] {
		childName := child.InheritedConstraints[constraintName]
		if childName == "" || !p.inheritedNamesKnown(child, childName) {
			return false
		}
	}
	return true
}

func (p *partitionIndexCreationPlan) markHierarchy(table *ir.Table, root string) {
	key := table.Schema + "." + table.Name
	p.roots[key] = root
	for _, child := range p.partitions[key] {
		p.markHierarchy(child, root)
	}
}

// hierarchyRoot returns the root key of a hierarchy with postponed
// constraints, or "" for any other table.
func (p *partitionIndexCreationPlan) hierarchyRoot(table *ir.Table) string {
	if p == nil {
		return ""
	}
	return p.roots[table.Schema+"."+table.Name]
}

func (p *partitionIndexCreationPlan) markUniqueTables(table *ir.Table) {
	key := table.Schema + "." + table.Name
	p.uniqueTables[key] = true
	for _, child := range p.partitions[key] {
		p.markUniqueTables(child)
	}
}

// defersConstraint reports whether the constraint is added after the
// partitions exist rather than inline with its table.
func (p *partitionIndexCreationPlan) defersConstraint(table *ir.Table, constraint *ir.Constraint) bool {
	if p == nil || table == nil || constraint == nil {
		return false
	}
	return p.constraints[table.Schema+"."+table.Name][constraint.Name]
}

// tableCreated records that the table's CREATE TABLE was emitted and adds the
// postponed constraints of every family that is now complete. Emitting them
// here, inside the table batch, keeps them ahead of SQL functions created
// after the batch whose bodies depend on the key (ON CONFLICT ON CONSTRAINT,
// GROUP BY functional dependency).
func (p *partitionIndexCreationPlan) tableCreated(table *ir.Table, targetSchema string, collector *diffCollector) {
	if p == nil {
		return
	}
	p.created[table.Schema+"."+table.Name] = true
	if p.hierarchyRoot(table) == "" {
		return
	}
	p.generateReadyConstraintSQL(targetSchema, collector, false)
}

// generateDeferredConstraintSQL adds any postponed constraint family not yet
// emitted by tableCreated.
func (p *partitionIndexCreationPlan) generateDeferredConstraintSQL(targetSchema string, collector *diffCollector) {
	if p == nil {
		return
	}
	p.generateReadyConstraintSQL(targetSchema, collector, true)
}

// generateReadyConstraintSQL adds the postponed PRIMARY KEY and UNIQUE
// constraints of each family whose table and partitions all exist (or of every
// remaining family when flush is set). ONLY keeps PostgreSQL from creating
// partition copies under generated names; each copy is added under its catalog
// name and attached explicitly. One step carries the whole family so the
// statements stay together in plans and multi-file dumps.
func (p *partitionIndexCreationPlan) generateReadyConstraintSQL(targetSchema string, collector *diffCollector, flush bool) {
	keys := make([]string, 0, len(p.constraints))
	for key := range p.constraints {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		table := p.tables[key]
		if !flush && !p.hierarchyCreated(table) {
			continue
		}
		for _, name := range sortedKeys(p.constraints[key]) {
			if p.emitted[key][name] {
				continue
			}
			if p.emitted[key] == nil {
				p.emitted[key] = make(map[string]bool)
			}
			p.emitted[key][name] = true
			constraint := table.Constraints[name]
			var statements []SQLStatement
			p.appendConstraintFamily(&statements, table, constraint, targetSchema, collector.qualifySchema)
			collector.collectStatements(&diffContext{
				Type: DiffTypeTableConstraint, Operation: DiffOperationCreate,
				Path:   fmt.Sprintf("%s.%s.%s", table.Schema, table.Name, constraint.Name),
				Source: constraint, CanRunInTransaction: true,
			}, statements)
		}
	}
}

// hierarchyCreated reports whether the table and every partition below it
// have been created.
func (p *partitionIndexCreationPlan) hierarchyCreated(table *ir.Table) bool {
	key := table.Schema + "." + table.Name
	if !p.created[key] {
		return false
	}
	for _, child := range p.partitions[key] {
		if !p.hierarchyCreated(child) {
			return false
		}
	}
	return true
}

// appendConstraintFamily adds the constraint to its table with ONLY, then each
// partition's copy under its recorded name, attaching every copy to the
// constraint's index. Nested partitions attach to their immediate parent.
func (p *partitionIndexCreationPlan) appendConstraintFamily(statements *[]SQLStatement, table *ir.Table, constraint *ir.Constraint, targetSchema string, qualifySchema bool) {
	tableName := qualifyEntityNameMode(table.Schema, table.Name, targetSchema, qualifySchema)
	*statements = append(*statements, SQLStatement{
		SQL:                 fmt.Sprintf("ALTER TABLE ONLY %s ADD %s;", tableName, generateConstraintSQL(constraint, targetSchema, qualifySchema)),
		CanRunInTransaction: true,
	})
	for _, child := range p.partitions[table.Schema+"."+table.Name] {
		childConstraint := *constraint
		childConstraint.Schema = child.Schema
		childConstraint.Table = child.Name
		childConstraint.Name = child.InheritedConstraints[constraint.Name]
		p.appendConstraintFamily(statements, child, &childConstraint, targetSchema, qualifySchema)
		*statements = append(*statements, SQLStatement{
			SQL: fmt.Sprintf("ALTER INDEX %s ATTACH PARTITION %s;",
				qualifyEntityNameMode(table.Schema, constraint.Name, targetSchema, qualifySchema),
				qualifyEntityNameMode(child.Schema, childConstraint.Name, targetSchema, qualifySchema)),
			CanRunInTransaction: true,
		})
	}
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
				Type: DiffTypeTableIndex, Operation: DiffOperationAttach,
				Path:   fmt.Sprintf("%s.%s.%s", index.Schema, index.Table, index.Name),
				Source: index, CanRunInTransaction: true,
			}, sql)
		}
	}
	for _, key := range keys {
		create(key)
	}
}
