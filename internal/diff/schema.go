package diff

import (
	"fmt"
	"sort"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateSchemasSQL emits CREATE SCHEMA IF NOT EXISTS (and optional AUTHORIZATION)
// for each namespace in addedSchemas before any object DDL.
func generateCreateSchemasSQL(schemas []*ir.Schema, collector *diffCollector) {
	if len(schemas) == 0 {
		return
	}
	sorted := append([]*ir.Schema(nil), schemas...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	for _, s := range sorted {
		if s == nil {
			continue
		}
		schemaLit := ir.QuoteIdentifier(s.Name)
		sql := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaLit)
		if s.Owner != "" {
			sql += fmt.Sprintf(" AUTHORIZATION %s", ir.QuoteIdentifier(s.Owner))
		}
		sql += ";"
		ctx := &diffContext{
			Type:                DiffTypeSchema,
			Operation:           DiffOperationCreate,
			Path:                s.Name,
			Source:              s,
			CanRunInTransaction: true,
		}
		collector.collect(ctx, sql)
	}
}

// generateDropSchemasSQL emits DROP SCHEMA IF EXISTS ... CASCADE for removed namespaces.
// CASCADE removes any objects still in the schema so the drop succeeds even if pgschema
// did not emit drops for every contained object (defensive; typical flows already dropped children).
func generateDropSchemasSQL(schemas []*ir.Schema, collector *diffCollector) {
	if len(schemas) == 0 {
		return
	}
	sorted := append([]*ir.Schema(nil), schemas...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name > sorted[j].Name
	})
	for _, s := range sorted {
		if s == nil {
			continue
		}
		sql := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE;", ir.QuoteIdentifier(s.Name))
		ctx := &diffContext{
			Type:                DiffTypeSchema,
			Operation:           DiffOperationDrop,
			Path:                s.Name,
			Source:              s,
			CanRunInTransaction: true,
		}
		collector.collect(ctx, sql)
	}
}
