package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreateColumnPrivilegesSQL generates GRANT statements for new column privileges
func generateCreateColumnPrivilegesSQL(privileges []*ir.ColumnPrivilege, targetSchema string, collector *diffCollector) {
	for _, cp := range privileges {
		sql := generateGrantColumnPrivilegeSQL(cp)

		// Path format: column_privileges.TABLE.{table_name}.{columns}.{grantee}
		sortedCols := make([]string, len(cp.Columns))
		copy(sortedCols, cp.Columns)
		sort.Strings(sortedCols)
		colKey := strings.Join(sortedCols, ",")

		context := &diffContext{
			Type:                DiffTypeColumnPrivilege,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("column_privileges.TABLE.%s.%s.%s", cp.TableName, colKey, cp.Grantee),
			Source:              cp,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateDropColumnPrivilegesSQL generates REVOKE statements for removed column privileges
func generateDropColumnPrivilegesSQL(privileges []*ir.ColumnPrivilege, targetSchema string, collector *diffCollector) {
	for _, cp := range privileges {
		sql := generateRevokeColumnPrivilegeSQL(cp)

		sortedCols := make([]string, len(cp.Columns))
		copy(sortedCols, cp.Columns)
		sort.Strings(sortedCols)
		colKey := strings.Join(sortedCols, ",")

		context := &diffContext{
			Type:                DiffTypeColumnPrivilege,
			Operation:           DiffOperationDrop,
			Path:                fmt.Sprintf("column_privileges.TABLE.%s.%s.%s", cp.TableName, colKey, cp.Grantee),
			Source:              cp,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateModifyColumnPrivilegesSQL generates ALTER column privilege statements for modifications
func generateModifyColumnPrivilegesSQL(diffs []*columnPrivilegeDiff, targetSchema string, collector *diffCollector) {
	for _, diff := range diffs {
		statements := diff.generateAlterColumnPrivilegeStatements()

		sortedCols := make([]string, len(diff.New.Columns))
		copy(sortedCols, diff.New.Columns)
		sort.Strings(sortedCols)
		colKey := strings.Join(sortedCols, ",")

		for _, stmt := range statements {
			context := &diffContext{
				Type:                DiffTypeColumnPrivilege,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("column_privileges.TABLE.%s.%s.%s", diff.New.TableName, colKey, diff.New.Grantee),
				Source:              diff,
				CanRunInTransaction: true,
			}

			collector.collect(context, stmt)
		}
	}
}

// generateGrantColumnPrivilegeSQL generates a GRANT statement for column privileges
func generateGrantColumnPrivilegeSQL(cp *ir.ColumnPrivilege) string {
	// Sort privileges for deterministic output
	sortedPrivs := make([]string, len(cp.Privileges))
	copy(sortedPrivs, cp.Privileges)
	sort.Strings(sortedPrivs)

	privStr := formatColumnPrivilegeList(sortedPrivs, formatColumnList(cp.Columns))

	grantee := formatGrantee(cp.Grantee)
	tableName := ir.QuoteIdentifier(cp.TableName)

	sql := fmt.Sprintf("GRANT %s ON TABLE %s TO %s", privStr, tableName, grantee)

	if cp.WithGrantOption {
		sql += " WITH GRANT OPTION"
	}

	return sql + ";"
}

// generateRevokeColumnPrivilegeSQL generates a REVOKE statement for column privileges
func generateRevokeColumnPrivilegeSQL(cp *ir.ColumnPrivilege) string {
	// Sort privileges for deterministic output
	sortedPrivs := make([]string, len(cp.Privileges))
	copy(sortedPrivs, cp.Privileges)
	sort.Strings(sortedPrivs)

	privStr := formatColumnPrivilegeList(sortedPrivs, formatColumnList(cp.Columns))

	grantee := formatGrantee(cp.Grantee)
	tableName := ir.QuoteIdentifier(cp.TableName)

	return fmt.Sprintf("REVOKE %s ON TABLE %s FROM %s;", privStr, tableName, grantee)
}

// generateAlterColumnPrivilegeStatements generates statements for column privilege modifications
func (d *columnPrivilegeDiff) generateAlterColumnPrivilegeStatements() []string {
	var statements []string

	// Find privileges to revoke (in old but not in new)
	oldPrivSet := make(map[string]bool)
	for _, p := range d.Old.Privileges {
		oldPrivSet[p] = true
	}
	newPrivSet := make(map[string]bool)
	for _, p := range d.New.Privileges {
		newPrivSet[p] = true
	}

	var toRevoke []string
	for p := range oldPrivSet {
		if !newPrivSet[p] {
			toRevoke = append(toRevoke, p)
		}
	}

	var toGrant []string
	for p := range newPrivSet {
		if !oldPrivSet[p] {
			toGrant = append(toGrant, p)
		}
	}

	grantee := formatGrantee(d.New.Grantee)
	tableName := ir.QuoteIdentifier(d.New.TableName)

	colStr := formatColumnList(d.New.Columns)

	// Generate REVOKE for removed privileges
	if len(toRevoke) > 0 {
		sort.Strings(toRevoke)
		statements = append(statements, fmt.Sprintf("REVOKE %s ON TABLE %s FROM %s;",
			formatColumnPrivilegeList(toRevoke, colStr), tableName, grantee))
	}

	// Generate GRANT for added privileges
	if len(toGrant) > 0 {
		sort.Strings(toGrant)
		sql := fmt.Sprintf("GRANT %s ON TABLE %s TO %s",
			formatColumnPrivilegeList(toGrant, colStr), tableName, grantee)
		if d.New.WithGrantOption {
			sql += " WITH GRANT OPTION"
		}
		statements = append(statements, sql+";")
	}

	// Handle WITH GRANT OPTION changes for unchanged privileges
	if d.Old.WithGrantOption != d.New.WithGrantOption {
		// Find unchanged privileges (in both old and new)
		var unchanged []string
		for p := range oldPrivSet {
			if newPrivSet[p] {
				unchanged = append(unchanged, p)
			}
		}

		if len(unchanged) > 0 {
			sort.Strings(unchanged)
			unchangedStr := formatColumnPrivilegeList(unchanged, colStr)

			if d.Old.WithGrantOption && !d.New.WithGrantOption {
				// Revoke grant option only (keep the privilege)
				statements = append(statements, fmt.Sprintf("REVOKE GRANT OPTION FOR %s ON TABLE %s FROM %s;",
					unchangedStr, tableName, grantee))
			} else if !d.Old.WithGrantOption && d.New.WithGrantOption {
				// Add grant option (re-grant with grant option)
				statements = append(statements, fmt.Sprintf("GRANT %s ON TABLE %s TO %s WITH GRANT OPTION;",
					unchangedStr, tableName, grantee))
			}
		}
	}

	return statements
}

// columnPrivilegesEqual checks if two column privileges are structurally equal
func columnPrivilegesEqual(old, new *ir.ColumnPrivilege) bool {
	if old.TableName != new.TableName {
		return false
	}
	if old.Grantee != new.Grantee {
		return false
	}
	if old.WithGrantOption != new.WithGrantOption {
		return false
	}

	// Compare columns (order-independent)
	if len(old.Columns) != len(new.Columns) {
		return false
	}
	oldColSet := make(map[string]bool)
	for _, c := range old.Columns {
		oldColSet[c] = true
	}
	for _, c := range new.Columns {
		if !oldColSet[c] {
			return false
		}
	}

	// Compare privileges (order-independent)
	if len(old.Privileges) != len(new.Privileges) {
		return false
	}
	oldPrivSet := make(map[string]bool)
	for _, p := range old.Privileges {
		oldPrivSet[p] = true
	}
	for _, p := range new.Privileges {
		if !oldPrivSet[p] {
			return false
		}
	}

	return true
}

// GetObjectName returns a unique identifier for the column privilege diff
func (d *columnPrivilegeDiff) GetObjectName() string {
	return d.New.GetObjectKey()
}

// formatColumnList quotes and sorts column names and joins them for use in a
// column privilege list, e.g. "a, b".
func formatColumnList(columns []string) string {
	quotedCols := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = ir.QuoteIdentifier(col)
	}
	sort.Strings(quotedCols)
	return strings.Join(quotedCols, ", ")
}

// formatColumnPrivilegeList attaches the column list to every privilege, e.g.
// "INSERT (a, b), UPDATE (a, b)". PostgreSQL applies a column list only to the
// privilege immediately before it, so "INSERT, UPDATE (a, b)" would grant
// INSERT on the whole table (#638).
func formatColumnPrivilegeList(privileges []string, colStr string) string {
	parts := make([]string, len(privileges))
	for i, p := range privileges {
		parts[i] = fmt.Sprintf("%s (%s)", p, colStr)
	}
	return strings.Join(parts, ", ")
}
