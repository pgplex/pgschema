package diff

import (
	"fmt"
	"sort"

	"github.com/pgplex/pgschema/ir"
)

// validatePartitionIndexDependencies rejects filtered index families before any
// dump statements are generated. Migration generation still honors filtering.
func validatePartitionIndexDependencies(schema *ir.IR) error {
	indexes := make(map[ir.IndexRef]*ir.Index)
	var keys []ir.IndexRef
	for _, namespace := range schema.Schemas {
		for _, table := range namespace.Tables {
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
	for _, key := range keys {
		for _, child := range indexes[key].AttachedPartitions {
			if _, exists := indexes[child]; !exists {
				return fmt.Errorf("cannot dump index %s: attached index %s is excluded; adjust .pgschemaignore to include the child index and its table",
					ir.QuoteIdentifier(key.Schema)+"."+ir.QuoteIdentifier(key.Name),
					ir.QuoteIdentifier(child.Schema)+"."+ir.QuoteIdentifier(child.Name))
			}
		}
	}
	return nil
}

// generatePartitionedIndexesSQL emits parent indexes after all partition tables
// and leaf indexes exist. ON ONLY prevents PostgreSQL from creating or choosing
// child indexes implicitly. Explicit attachments preserve names and validity.
func generatePartitionedIndexesSQL(tables []*ir.Table, targetSchema string, collector *diffCollector) {
	type indexKey struct{ schema, name string }
	parents := make(map[indexKey]*ir.Index)
	children := make(map[indexKey][]*ir.Index)
	var keys []indexKey
	for _, table := range tables {
		for _, index := range table.Indexes {
			if index.Type == ir.IndexTypePrimary {
				continue // Primary indexes are created by constraints.
			}
			key := indexKey{index.Schema, index.Name}
			if index.IsPartitioned {
				parents[key] = index
				keys = append(keys, key)
			}
			if index.ParentIndex != "" {
				parent := indexKey{index.ParentIndexSchema, index.ParentIndex}
				children[parent] = append(children[parent], index)
			}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].schema != keys[j].schema {
			return keys[i].schema < keys[j].schema
		}
		return keys[i].name < keys[j].name
	})
	created := make(map[indexKey]bool)
	var create func(indexKey)
	create = func(key indexKey) {
		if created[key] {
			return
		}
		created[key] = true
		attached := children[key]
		sort.Slice(attached, func(i, j int) bool {
			if attached[i].Schema != attached[j].Schema {
				return attached[i].Schema < attached[j].Schema
			}
			return attached[i].Name < attached[j].Name
		})
		// A partitioned child must exist before its own attachment is emitted.
		for _, child := range attached {
			if child.IsPartitioned {
				create(indexKey{child.Schema, child.Name})
			}
		}
		index := parents[key]
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

// referencesPartitionedUniqueIndex identifies FKs that may need a postponed
// unique index. Those FKs must wait until the index and its attachments exist.
func referencesPartitionedUniqueIndex(table *ir.Table, constraint *ir.Constraint, tables map[string]*ir.Table) bool {
	if constraint == nil || constraint.Type != ir.ConstraintTypeForeignKey {
		return false
	}
	schema := constraint.ReferencedSchema
	if schema == "" {
		schema = table.Schema
	}
	parent := tables[schema+"."+constraint.ReferencedTable]
	if parent == nil || !parent.IsPartitioned {
		return false
	}
	for _, index := range parent.Indexes {
		if index.Type == ir.IndexTypeUnique {
			return true
		}
	}
	return false
}
