package diff

import (
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateColumnSQL generates SQL statements for column modifications
func (cd *ColumnDiff) generateColumnSQL(tableSchema, tableName string, targetSchema string) []string {
	var statements []string
	qualifiedTableName := getTableNameWithSchema(tableSchema, tableName, targetSchema)

	// Handle data type changes - normalize types by stripping target schema prefix.
	// Base type names (without modifiers) drive cast-compatibility decisions,
	// while the full type strings (with numeric precision/scale, varchar length)
	// drive change detection and the emitted TYPE clause - otherwise a
	// numeric(18,6) -> numeric(20,6) change would be invisible here.
	oldBaseType := stripSchemaPrefix(cd.Old.DataType, targetSchema)
	newBaseType := stripSchemaPrefix(cd.New.DataType, targetSchema)
	oldType := stripSchemaPrefix(comparableColumnType(cd.Old), targetSchema)
	newType := stripSchemaPrefix(comparableColumnType(cd.New), targetSchema)

	// Check if there's a type change AND the column has a default value
	// When a USING clause is needed, we must: DROP DEFAULT -> ALTER TYPE -> SET DEFAULT
	// because PostgreSQL can't automatically cast default values during type changes with USING
	hasTypeChange := oldType != newType
	oldDefault := cd.Old.DefaultValue
	newDefault := cd.New.DefaultValue
	hasOldDefault := oldDefault != nil && *oldDefault != ""
	needsUsing := hasTypeChange && needsUsingClause(oldBaseType, newBaseType)

	// If type is changing with USING clause and there's an existing default, drop the default first
	if needsUsing && hasOldDefault {
		sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
			qualifiedTableName, ir.QuoteIdentifier(cd.New.Name))
		statements = append(statements, sql)
	}

	// Handle data type changes
	if hasTypeChange {
		// Check if we need a USING clause for the type conversion
		// This is required when converting from text-like types to custom types (like ENUMs)
		// because PostgreSQL cannot implicitly cast these types
		if needsUsing {
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name), newType, ir.QuoteIdentifier(cd.New.Name), newType)
			statements = append(statements, sql)
		} else {
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name), newType)
			statements = append(statements, sql)
		}
	}

	// Handle nullable changes
	if cd.Old.IsNullable != cd.New.IsNullable {
		if cd.New.IsNullable {
			// DROP NOT NULL
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name))
			statements = append(statements, sql)
		} else {
			// ADD NOT NULL - generate canonical SQL only
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name))
			statements = append(statements, sql)
		}
	}

	// Handle default value changes
	// When USING clause was needed, we dropped the default above, so re-add it if there's a new default
	// When USING clause was NOT needed, handle default changes normally
	if needsUsing && hasOldDefault {
		// Default was dropped above; add new default if specified
		if newDefault != nil && *newDefault != "" {
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name), *newDefault)
			statements = append(statements, sql)
		}
	} else {
		// Normal default value change handling (no USING clause involved)
		// We only drop default values when they are not sequences
		// Sequences are automatically handled by the DROP CASCADE statement
		if oldDefault != nil && newDefault == nil && !strings.HasPrefix(*oldDefault, "nextval(") {
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name))
			statements = append(statements, sql)
		}
		if (oldDefault == nil && newDefault != nil) || (oldDefault != nil && newDefault != nil && *oldDefault != *newDefault) {
			sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;",
				qualifiedTableName, ir.QuoteIdentifier(cd.New.Name), *newDefault)
			statements = append(statements, sql)
		}
	}

	// Handle identity column changes
	if cd.Old.Identity != nil && (cd.New.Identity == nil || cd.Old.Identity.Generation != cd.New.Identity.Generation) {
		sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP IDENTITY;",
			qualifiedTableName, ir.QuoteIdentifier(cd.New.Name))
		statements = append(statements, sql)
	}
	if cd.New.Identity != nil && (cd.Old.Identity == nil || cd.Old.Identity.Generation != cd.New.Identity.Generation) {
		sql := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED %s AS IDENTITY;",
			qualifiedTableName, ir.QuoteIdentifier(cd.New.Name), cd.New.Identity.Generation)
		statements = append(statements, sql)
	}

	return statements
}

// needsUsingClause determines if a type conversion requires a USING clause.
//
// This is especially important when converting to or from custom types (like ENUMs),
// because PostgreSQL often cannot implicitly cast these types. To avoid generating
// invalid migrations, this function takes a conservative approach:
//
//   - Any conversion involving at least one non–built-in (custom) type will require
//     a USING clause.
//   - For built-in → built-in conversions we still assume PostgreSQL provides an
//     implicit cast in most cases; callers should be aware that some edge cases
//     (e.g. certain text → json conversions) may still need manual adjustment.
func needsUsingClause(oldType, newType string) bool {
	// Check if old type is text-like
	oldIsTextLike := ir.IsTextLikeType(oldType)

	// Determine whether the old/new types are PostgreSQL built-ins
	oldIsBuiltIn := ir.IsBuiltInType(oldType)
	newIsBuiltIn := ir.IsBuiltInType(newType)

	// Preserve existing behavior: text-like → non–built-in likely needs USING
	if oldIsTextLike && !newIsBuiltIn {
		return true
	}

	// Be conservative for any conversion involving custom (non–built-in) types:
	// this covers custom → custom and built-in ↔ custom conversions.
	if !oldIsBuiltIn || !newIsBuiltIn {
		return true
	}

	// For built-in → built-in types we assume an implicit cast is available.
	return false
}

// comparableColumnType returns the column's data type including any
// precision/scale/length modifiers, for use in type-change detection and
// the emitted ALTER ... TYPE clause. Unlike formatColumnDataType it does
// not expand SERIAL: serial is integer + sequence-default sugar, and
// serial<->integer/identity transitions are handled by the sequence and
// identity machinery rather than a TYPE change.
func comparableColumnType(column *ir.Column) string {
	if isSerialColumn(column) {
		return column.DataType
	}
	return formatColumnDataType(column)
}

// columnsEqual compares two columns for equality
// targetSchema is used to normalize type names before comparison
func columnsEqual(old, new *ir.Column, targetSchema string) bool {
	if old.Name != new.Name {
		return false
	}
	// Normalize types by stripping target schema prefix before comparison.
	// Use comparableColumnType so type modifiers (numeric precision/scale,
	// varchar length) are included - DataType alone is just "numeric" for
	// both numeric(18,6) and numeric(20,6).
	oldType := stripSchemaPrefix(comparableColumnType(old), targetSchema)
	newType := stripSchemaPrefix(comparableColumnType(new), targetSchema)
	if oldType != newType {
		return false
	}
	if old.IsNullable != new.IsNullable {
		return false
	}

	// Compare default values (already normalized by ir.normalizeColumn)
	if (old.DefaultValue == nil) != (new.DefaultValue == nil) {
		return false
	}
	if old.DefaultValue != nil && new.DefaultValue != nil && *old.DefaultValue != *new.DefaultValue {
		return false
	}

	// Compare max length
	if (old.MaxLength == nil) != (new.MaxLength == nil) {
		return false
	}
	if old.MaxLength != nil && new.MaxLength != nil && *old.MaxLength != *new.MaxLength {
		return false
	}

	// Compare identity columns
	if (old.Identity == nil) != (new.Identity == nil) {
		return false
	}
	if old.Identity != nil && new.Identity != nil {
		if old.Identity.Generation != new.Identity.Generation {
			return false
		}
	}

	// Compare comments
	if old.Comment != new.Comment {
		return false
	}

	return true
}
