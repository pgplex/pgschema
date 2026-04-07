package diff

import (
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateConstraintSQL generates constraint definition for inline table constraints
func generateConstraintSQL(constraint *ir.Constraint, targetSchema string) string {
	// Helper function to get column names from ConstraintColumn array
	getColumnNames := func(columns []*ir.ConstraintColumn) []string {
		var names []string
		for _, col := range columns {
			names = append(names, ir.QuoteIdentifier(col.Name))
		}
		return names
	}

	switch constraint.Type {
	case ir.ConstraintTypePrimaryKey:
		// Always include CONSTRAINT name to be explicit and consistent
		cols := getColumnNames(constraint.Columns)
		if constraint.IsTemporal && len(cols) > 0 {
			cols[len(cols)-1] = cols[len(cols)-1] + " WITHOUT OVERLAPS"
		}
		return fmt.Sprintf("CONSTRAINT %s PRIMARY KEY (%s)", ir.QuoteIdentifier(constraint.Name), strings.Join(cols, ", "))
	case ir.ConstraintTypeUnique:
		// Always include CONSTRAINT name to be explicit and consistent
		cols := getColumnNames(constraint.Columns)
		if constraint.IsTemporal && len(cols) > 0 {
			cols[len(cols)-1] = cols[len(cols)-1] + " WITHOUT OVERLAPS"
		}
		return fmt.Sprintf("CONSTRAINT %s UNIQUE (%s)", ir.QuoteIdentifier(constraint.Name), strings.Join(cols, ", "))
	case ir.ConstraintTypeForeignKey:
		// Always include CONSTRAINT name to preserve explicit FK names
		// Use QualifyEntityNameWithQuotes to add schema qualifier when referencing tables in other schemas
		cols := getColumnNames(constraint.Columns)
		refCols := getColumnNames(constraint.ReferencedColumns)
		if constraint.IsTemporal {
			if len(cols) > 0 {
				cols[len(cols)-1] = "PERIOD " + cols[len(cols)-1]
			}
			if len(refCols) > 0 {
				refCols[len(refCols)-1] = "PERIOD " + refCols[len(refCols)-1]
			}
		}
		qualifiedRefTable := ir.QualifyEntityNameWithQuotes(constraint.ReferencedSchema, constraint.ReferencedTable, targetSchema)
		stmt := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)",
			ir.QuoteIdentifier(constraint.Name),
			strings.Join(cols, ", "),
			qualifiedRefTable, strings.Join(refCols, ", "))
		// Only add ON UPDATE/DELETE if they are not the default "NO ACTION"
		if constraint.UpdateRule != "" && constraint.UpdateRule != "NO ACTION" {
			stmt += fmt.Sprintf(" ON UPDATE %s", constraint.UpdateRule)
		}
		if constraint.DeleteRule != "" && constraint.DeleteRule != "NO ACTION" {
			stmt += fmt.Sprintf(" ON DELETE %s", constraint.DeleteRule)
		}
		// Add deferrable clause
		if constraint.Deferrable {
			if constraint.InitiallyDeferred {
				stmt += " DEFERRABLE INITIALLY DEFERRED"
			} else {
				stmt += " DEFERRABLE"
			}
		}
		// Add NOT VALID if needed
		if !constraint.IsValid {
			stmt += " NOT VALID"
		}
		return stmt
	case ir.ConstraintTypeCheck:
		// Generate CHECK constraint with proper NOT VALID / NO INHERIT placement
		// The CheckClause is normalized to exclude NOT VALID and NO INHERIT (stripped in normalize.go)
		// We append them based on IsValid/NoInherit fields, mimicking pg_dump behavior
		result := fmt.Sprintf("CONSTRAINT %s %s", ir.QuoteIdentifier(constraint.Name), constraint.CheckClause)
		if constraint.NoInherit {
			result += " NO INHERIT"
		}
		if !constraint.IsValid {
			result += " NOT VALID"
		}
		return result
	case ir.ConstraintTypeExclusion:
		// Use the full definition from pg_get_constraintdef()
		return fmt.Sprintf("CONSTRAINT %s %s", ir.QuoteIdentifier(constraint.Name), constraint.ExclusionDefinition)
	default:
		return ""
	}
}

// getInlineConstraintsForTable returns constraints in the correct order: PRIMARY KEY, UNIQUE, FOREIGN KEY
func getInlineConstraintsForTable(table *ir.Table) []*ir.Constraint {
	var inlineConstraints []*ir.Constraint

	// Get constraint names sorted for consistent output (sorting handled by IR)
	constraintNames := sortedKeys(table.Constraints)

	// Separate constraints by type for proper ordering
	var primaryKeys []*ir.Constraint
	var uniques []*ir.Constraint
	var foreignKeys []*ir.Constraint
	var checkConstraints []*ir.Constraint
	var exclusionConstraints []*ir.Constraint

	for _, constraintName := range constraintNames {
		constraint := table.Constraints[constraintName]

		// Categorize constraints by type
		// ALL constraints are now included as table-level constraints for consistency
		// This ensures all constraint names are preserved and provides cleaner formatting
		switch constraint.Type {
		case ir.ConstraintTypePrimaryKey:
			primaryKeys = append(primaryKeys, constraint)
		case ir.ConstraintTypeUnique:
			uniques = append(uniques, constraint)
		case ir.ConstraintTypeForeignKey:
			foreignKeys = append(foreignKeys, constraint)
		case ir.ConstraintTypeCheck:
			checkConstraints = append(checkConstraints, constraint)
		case ir.ConstraintTypeExclusion:
			exclusionConstraints = append(exclusionConstraints, constraint)
		}
	}

	// Add constraints in order: PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK, EXCLUDE
	inlineConstraints = append(inlineConstraints, primaryKeys...)
	inlineConstraints = append(inlineConstraints, uniques...)
	inlineConstraints = append(inlineConstraints, foreignKeys...)
	inlineConstraints = append(inlineConstraints, checkConstraints...)
	inlineConstraints = append(inlineConstraints, exclusionConstraints...)

	return inlineConstraints
}

// constraintsEqual compares two constraints for equality
func constraintsEqual(old, new *ir.Constraint) bool {
	// Basic properties
	if old.Name != new.Name {
		return false
	}
	if old.Type != new.Type {
		return false
	}
	if old.ReferencedSchema != new.ReferencedSchema {
		return false
	}
	if old.ReferencedTable != new.ReferencedTable {
		return false
	}
	if old.CheckClause != new.CheckClause {
		return false
	}
	if old.NoInherit != new.NoInherit {
		return false
	}
	if old.ExclusionDefinition != new.ExclusionDefinition {
		return false
	}

	// Foreign key specific properties (this is the key fix!)
	if old.DeleteRule != new.DeleteRule {
		return false
	}
	if old.UpdateRule != new.UpdateRule {
		return false
	}
	if old.Deferrable != new.Deferrable {
		return false
	}
	if old.InitiallyDeferred != new.InitiallyDeferred {
		return false
	}
	if old.IsTemporal != new.IsTemporal {
		return false
	}

	// Validation status - only compare for CHECK and FOREIGN KEY constraints
	// PRIMARY KEY and UNIQUE constraints are always valid (IsValid is not meaningful for them)
	if old.Type == ir.ConstraintTypeCheck || old.Type == ir.ConstraintTypeForeignKey {
		if old.IsValid != new.IsValid {
			return false
		}
	}

	// Comments
	if old.Comment != new.Comment {
		return false
	}

	// Compare columns (skip for CHECK and EXCLUDE constraints as column detection may differ)
	if old.Type != ir.ConstraintTypeCheck && old.Type != ir.ConstraintTypeExclusion {
		if len(old.Columns) != len(new.Columns) {
			return false
		}
		for i, oldCol := range old.Columns {
			newCol := new.Columns[i]
			if oldCol.Name != newCol.Name || oldCol.Position != newCol.Position {
				return false
			}
		}
	}

	// Compare referenced columns
	if len(old.ReferencedColumns) != len(new.ReferencedColumns) {
		return false
	}
	for i, oldCol := range old.ReferencedColumns {
		newCol := new.ReferencedColumns[i]
		if oldCol.Name != newCol.Name || oldCol.Position != newCol.Position {
			return false
		}
	}

	return true
}
