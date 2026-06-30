package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateTypesSQL generates CREATE TYPE statements
func generateCreateTypesSQL(types []*ir.Type, targetSchema string, collector *diffCollector) {
	// Sort types topologically to handle dependencies (e.g., composite types referencing enum types)
	sortedTypes := topologicallySortTypes(types)

	for _, typeObj := range sortedTypes {
		sql := generateTypeSQL(typeObj, targetSchema, collector.qualifySchema)

		// Determine DiffType based on type kind

		// Create context for this statement
		var diffType DiffType
		if typeObj.Kind == ir.TypeKindDomain {
			diffType = DiffTypeDomain
		} else {
			diffType = DiffTypeType
		}

		context := &diffContext{
			Type:                diffType,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s", typeObj.Schema, typeObj.Name),
			Source:              typeObj,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateModifyTypesSQL generates ALTER TYPE statements
func generateModifyTypesSQL(diffs []*typeDiff, targetSchema string, collector *diffCollector) {
	for _, diff := range diffs {
		switch diff.Old.Kind {
		case ir.TypeKindEnum:
			// ENUM types can be modified by adding values
			if diff.New.Kind == ir.TypeKindEnum {
				alterStatements := generateAlterTypeEnumStatements(diff.Old, diff.New, targetSchema)
				for _, stmt := range alterStatements {
					context := &diffContext{
						Type:                DiffTypeType,
						Operation:           DiffOperationAlter,
						Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
						Source:              diff,
						CanRunInTransaction: true,
					}
					collector.collect(context, stmt)
				}
			}
		case ir.TypeKindDomain:
			// Domain types can be modified with ALTER DOMAIN
			if diff.New.Kind == ir.TypeKindDomain {
				alterStatements := generateAlterDomainStatements(diff.Old, diff.New, targetSchema)
				for _, stmt := range alterStatements {
					context := &diffContext{
						Type:                DiffTypeDomain,
						Operation:           DiffOperationAlter,
						Path:                fmt.Sprintf("%s.%s", diff.New.Schema, diff.New.Name),
						Source:              diff,
						CanRunInTransaction: true,
					}
					collector.collect(context, stmt)
				}
			}
		}
	}
}

// generateDropTypesSQL generates DROP TYPE statements
func generateDropTypesSQL(types []*ir.Type, targetSchema string, collector *diffCollector) {
	// Sort types by name for consistent ordering
	sortedTypes := make([]*ir.Type, len(types))
	copy(sortedTypes, types)
	sort.Slice(sortedTypes, func(i, j int) bool {
		return sortedTypes[i].Name < sortedTypes[j].Name
	})

	for _, typeObj := range sortedTypes {
		typeName := qualifyEntityName(typeObj.Schema, typeObj.Name, targetSchema)

		var sql string
		if typeObj.Kind == ir.TypeKindDomain {
			sql = fmt.Sprintf("DROP DOMAIN IF EXISTS %s RESTRICT;", typeName)
		} else {
			sql = fmt.Sprintf("DROP TYPE IF EXISTS %s RESTRICT;", typeName)
		}

		// Create context for this statement
		var diffType DiffType
		if typeObj.Kind == ir.TypeKindDomain {
			diffType = DiffTypeDomain
		} else {
			diffType = DiffTypeType
		}

		context := &diffContext{
			Type:                diffType,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("%s.%s", typeObj.Schema, typeObj.Name),
			Source:              typeObj,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateAlterTypeEnumStatements generates ALTER TYPE ADD VALUE statements for enum changes
func generateAlterTypeEnumStatements(oldType, newType *ir.Type, targetSchema string) []string {
	var statements []string

	// Create a map of old enum values for quick lookup
	oldValues := make(map[string]int)
	for i, value := range oldType.EnumValues {
		oldValues[value] = i
	}

	// Find new values and their positions
	typeName := qualifyEntityName(newType.Schema, newType.Name, targetSchema)

	for i, newValue := range newType.EnumValues {
		if _, exists := oldValues[newValue]; !exists {
			// This is a new value, generate ALTER TYPE ADD VALUE statement
			var stmt string
			if i == 0 {
				// Add at the beginning
				stmt = fmt.Sprintf("ALTER TYPE %s ADD VALUE '%s' BEFORE '%s';", typeName, newValue, newType.EnumValues[1])
			} else if i == len(newType.EnumValues)-1 {
				// Add at the end
				stmt = fmt.Sprintf("ALTER TYPE %s ADD VALUE '%s' AFTER '%s';", typeName, newValue, newType.EnumValues[i-1])
			} else {
				// Add in the middle
				stmt = fmt.Sprintf("ALTER TYPE %s ADD VALUE '%s' AFTER '%s';", typeName, newValue, newType.EnumValues[i-1])
			}
			statements = append(statements, stmt)
		}
	}

	return statements
}

// generateAlterDomainStatements generates ALTER DOMAIN statements for domain changes
func generateAlterDomainStatements(oldDomain, newDomain *ir.Type, targetSchema string) []string {
	var statements []string
	domainName := qualifyEntityName(newDomain.Schema, newDomain.Name, targetSchema)

	// Check if default value changed
	if oldDomain.Default != newDomain.Default {
		if newDomain.Default == "" {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP DEFAULT;", domainName))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s SET DEFAULT %s;", domainName, newDomain.Default))
		}
	}

	// Check if NOT NULL changed
	if oldDomain.NotNull != newDomain.NotNull {
		if newDomain.NotNull {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s SET NOT NULL;", domainName))
		} else {
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP NOT NULL;", domainName))
		}
	}

	// Check constraints changes
	// Create maps for easier comparison
	oldConstraints := make(map[string]*ir.DomainConstraint)
	for _, c := range oldDomain.Constraints {
		key := c.Name
		if key == "" {
			key = c.Definition
		}
		oldConstraints[key] = c
	}

	newConstraints := make(map[string]*ir.DomainConstraint)
	for _, c := range newDomain.Constraints {
		key := c.Name
		if key == "" {
			key = c.Definition
		}
		newConstraints[key] = c
	}

	// Drop removed constraints
	for key, oldConstraint := range oldConstraints {
		if newConstraint, exists := newConstraints[key]; !exists {
			// Constraint was removed
			if oldConstraint.Name != "" {
				statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP CONSTRAINT %s;", domainName, ir.QuoteIdentifier(oldConstraint.Name)))
			}
			// Note: unnamed constraints cannot be dropped individually
		} else if oldConstraint.Name != "" && oldConstraint.Definition != newConstraint.Definition {
			// Constraint exists but definition changed - need to drop and recreate
			statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s DROP CONSTRAINT %s;", domainName, ir.QuoteIdentifier(oldConstraint.Name)))
		}
	}

	// Add new constraints
	for key, newConstraint := range newConstraints {
		oldConstraint, exists := oldConstraints[key]
		if !exists || (exists && oldConstraint.Definition != newConstraint.Definition) {
			// Either new constraint or definition changed
			constraintDef := newConstraint.Definition
			if newConstraint.Name != "" {
				statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s ADD CONSTRAINT %s %s;", domainName, ir.QuoteIdentifier(newConstraint.Name), constraintDef))
			} else {
				statements = append(statements, fmt.Sprintf("ALTER DOMAIN %s ADD %s;", domainName, constraintDef))
			}
		}
	}

	return statements
}

// generateTypeSQL generates CREATE TYPE statement
func generateTypeSQL(typeObj *ir.Type, targetSchema string, qualifySchema bool) string {
	// Only include type name without schema if it's in the target schema (unless
	// forced qualification is on).
	typeName := qualifyEntityNameMode(typeObj.Schema, typeObj.Name, targetSchema, qualifySchema)

	switch typeObj.Kind {
	case ir.TypeKindEnum:
		if len(typeObj.EnumValues) == 0 {
			return fmt.Sprintf("CREATE TYPE %s AS ENUM ();", typeName)
		}

		// Use multi-line format for better readability
		var lines []string
		lines = append(lines, fmt.Sprintf("CREATE TYPE %s AS ENUM (", typeName))
		for i, value := range typeObj.EnumValues {
			if i == len(typeObj.EnumValues)-1 {
				// Last value, no comma
				lines = append(lines, fmt.Sprintf("    '%s'", value))
			} else {
				// Not last value, add comma
				lines = append(lines, fmt.Sprintf("    '%s',", value))
			}
		}
		lines = append(lines, ");")
		return strings.Join(lines, "\n")
	case ir.TypeKindComposite:
		var attributes []string
		for _, attr := range typeObj.Columns {
			// Strip schema prefix from data type if it matches the target schema
			dataType := stripSchemaPrefixMode(attr.DataType, targetSchema, qualifySchema)
			attributes = append(attributes, fmt.Sprintf("%s %s", attr.Name, dataType))
		}
		return fmt.Sprintf("CREATE TYPE %s AS (%s);", typeName, strings.Join(attributes, ", "))
	case ir.TypeKindDomain:
		// Keep the base type consistent with composite attribute types: preserve a
		// cross-schema (or target-schema) prefix under forced qualification, strip the
		// target-schema prefix otherwise. Note: same-schema user-defined base types are
		// stored bare by the inspector, so this cannot *add* a prefix to them.
		baseType := stripSchemaPrefixMode(typeObj.BaseType, targetSchema, qualifySchema)

		// Use multi-line format for better readability if there are constraints
		hasConstraints := len(typeObj.Constraints) > 0 || typeObj.NotNull || typeObj.Default != ""

		if !hasConstraints {
			return fmt.Sprintf("CREATE DOMAIN %s AS %s;", typeName, baseType)
		}

		// Multi-line format
		lines := []string{fmt.Sprintf("CREATE DOMAIN %s AS %s", typeName, baseType)}

		if typeObj.Default != "" {
			lines = append(lines, fmt.Sprintf("  DEFAULT %s", typeObj.Default))
		}
		if typeObj.NotNull {
			lines = append(lines, "  NOT NULL")
		}

		// Add domain constraints (CHECK constraints)
		// Normalize VALUE to uppercase for consistency
		for _, constraint := range typeObj.Constraints {
			constraintDef := constraint.Definition
			if constraint.Name != "" {
				lines = append(lines, fmt.Sprintf("  CONSTRAINT %s %s", ir.QuoteIdentifier(constraint.Name), constraintDef))
			} else {
				lines = append(lines, fmt.Sprintf("  %s", constraintDef))
			}
		}

		return strings.Join(lines, "\n") + ";"
	default:
		return fmt.Sprintf("CREATE TYPE %s;", typeName)
	}
}

// typesEqual compares two types for equality
func typesEqual(old, new *ir.Type) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if old.Kind != new.Kind {
		return false
	}

	switch old.Kind {
	case ir.TypeKindEnum:
		// For ENUM types, compare values
		if len(old.EnumValues) != len(new.EnumValues) {
			return false
		}
		for i, value := range old.EnumValues {
			if value != new.EnumValues[i] {
				return false
			}
		}

	case ir.TypeKindComposite:
		// For composite types, compare columns
		if len(old.Columns) != len(new.Columns) {
			return false
		}
		for i, col := range old.Columns {
			newCol := new.Columns[i]
			if col.Name != newCol.Name || col.DataType != newCol.DataType {
				return false
			}
		}

	case ir.TypeKindDomain:
		// For domain types, compare base type and constraints
		if old.BaseType != new.BaseType {
			return false
		}
		if old.NotNull != new.NotNull {
			return false
		}
		if old.Default != new.Default {
			return false
		}
		if len(old.Constraints) != len(new.Constraints) {
			return false
		}
		for i, constraint := range old.Constraints {
			newConstraint := new.Constraints[i]
			if constraint.Name != newConstraint.Name || constraint.Definition != newConstraint.Definition {
				return false
			}
		}
	}

	return true
}
