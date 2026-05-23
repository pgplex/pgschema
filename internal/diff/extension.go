package diff

import (
	"fmt"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateExtensionsSQL generates `CREATE EXTENSION IF NOT EXISTS` statements
// for newly added extensions. Emitted before any schema-level objects because
// extensions can provide operator classes, types, and functions that those
// objects depend on (e.g., a GIST index using btree_gist's UUID operator class).
func generateCreateExtensionsSQL(extensions []*ir.Extension, collector *diffCollector) {
	for _, ext := range extensions {
		sql := generateExtensionSQL(ext)
		context := &diffContext{
			Type:                DiffTypeExtension,
			Operation:           DiffOperationCreate,
			Path:                extensionPath(ext),
			Source:              ext,
			CanRunInTransaction: true,
		}
		collector.collect(context, sql)
	}
}

// generateDropExtensionsSQL generates `DROP EXTENSION IF EXISTS` statements
// for extensions removed from the target. Emitted after all schema-level drops
// to avoid dependency conflicts.
func generateDropExtensionsSQL(extensions []*ir.Extension, collector *diffCollector) {
	for _, ext := range extensions {
		context := &diffContext{
			Type:                DiffTypeExtension,
			Operation:           DiffOperationDrop,
			Path:                extensionPath(ext),
			Source:              ext,
			CanRunInTransaction: true,
		}
		collector.collect(context, fmt.Sprintf("DROP EXTENSION IF EXISTS %s;", ir.QuoteIdentifier(ext.Name)))
	}
}

// extensionPath returns the identifier used in the diff Path field. Extensions
// are cluster-level so no schema qualifier is included; doing so would leak
// the plan command's temporary schema into the recorded plan and break
// golden-output stability across runs.
func extensionPath(ext *ir.Extension) string {
	return ext.Name
}

// generateExtensionSQL renders a single CREATE EXTENSION statement.
// Extensions are cluster-level; the installed schema is intentionally not
// emitted here. Honoring it would require either pinning it to the user's
// declared value (which we cannot recover from pg_extension alone — the plan
// command's temporary schema becomes the install schema when no WITH SCHEMA
// is given) or filtering out transient schemas. Preserving the user-declared
// install schema is tracked as a follow-up to #436.
func generateExtensionSQL(ext *ir.Extension) string {
	return fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s;", ir.QuoteIdentifier(ext.Name))
}
