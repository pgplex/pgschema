package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pgplex/pgschema/ir"
)

// generateCreatePoliciesSQL generates CREATE POLICY statements
func generateCreatePoliciesSQL(policies []*ir.RLSPolicy, targetSchema string, collector *diffCollector) {
	// Sort policies by (schema, table, name) for deterministic ordering across tables.
	// Policy names are only unique per table, so sorting by name alone is insufficient
	// when policies from multiple tables are collected together (#373).
	sortedPolicies := make([]*ir.RLSPolicy, len(policies))
	copy(sortedPolicies, policies)
	sort.Slice(sortedPolicies, func(i, j int) bool {
		if sortedPolicies[i].Schema != sortedPolicies[j].Schema {
			return sortedPolicies[i].Schema < sortedPolicies[j].Schema
		}
		if sortedPolicies[i].Table != sortedPolicies[j].Table {
			return sortedPolicies[i].Table < sortedPolicies[j].Table
		}
		return sortedPolicies[i].Name < sortedPolicies[j].Name
	})

	for _, policy := range sortedPolicies {
		sql := generatePolicySQL(policy, targetSchema)

		// Create context for this statement
		context := &diffContext{
			Type:                DiffTypeTablePolicy,
			Operation:           DiffOperationCreate,
			Path:                fmt.Sprintf("%s.%s.%s", policy.Schema, policy.Table, policy.Name),
			Source:              policy,
			CanRunInTransaction: true,
		}

		collector.collect(context, sql)
	}
}

// generateRLSChangesSQL generates RLS enable/disable and force statements
func generateRLSChangesSQL(changes []*rlsChange, targetSchema string, collector *diffCollector) {
	for _, change := range changes {
		tableName := qualifyEntityName(change.Table.Schema, change.Table.Name, targetSchema)

		// Handle ENABLE/DISABLE changes
		if change.Enabled != nil {
			var sql string
			if *change.Enabled {
				sql = fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", tableName)
			} else {
				sql = fmt.Sprintf("ALTER TABLE %s DISABLE ROW LEVEL SECURITY;", tableName)
			}

			context := &diffContext{
				Type:                DiffTypeTableRLS,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", change.Table.Schema, change.Table.Name),
				Source:              change,
				CanRunInTransaction: true,
			}

			collector.collect(context, sql)
		}

		// Handle FORCE/NO FORCE changes
		if change.Forced != nil {
			var sql string
			if *change.Forced {
				sql = fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY;", tableName)
			} else {
				sql = fmt.Sprintf("ALTER TABLE %s NO FORCE ROW LEVEL SECURITY;", tableName)
			}

			context := &diffContext{
				Type:                DiffTypeTableRLS,
				Operation:           DiffOperationAlter,
				Path:                fmt.Sprintf("%s.%s", change.Table.Schema, change.Table.Name),
				Source:              change,
				CanRunInTransaction: true,
			}

			collector.collect(context, sql)
		}
	}
}

// generatePolicySQL generates CREATE POLICY statement
func generatePolicySQL(policy *ir.RLSPolicy, targetSchema string) string {
	// Only include table name without schema if it's in the target schema
	tableName := getTableNameWithSchema(policy.Schema, policy.Table, targetSchema)

	policyStmt := fmt.Sprintf("CREATE POLICY %s ON %s", ir.QuoteIdentifier(policy.Name), tableName)

	// Add command type if specified
	if policy.Command != ir.PolicyCommandAll {
		policyStmt += fmt.Sprintf(" FOR %s", policy.Command)
	}

	// Add roles if specified
	if len(policy.Roles) > 0 {
		policyStmt += " TO "
		for i, role := range policy.Roles {
			if i > 0 {
				policyStmt += ", "
			}
			policyStmt += role
		}
	}

	// Add USING clause if present
	if policy.Using != "" {
		policyStmt += fmt.Sprintf(" USING %s", ensureParentheses(policy.Using))
	}

	// Add WITH CHECK clause if present
	if policy.WithCheck != "" {
		policyStmt += fmt.Sprintf(" WITH CHECK %s", ensureParentheses(policy.WithCheck))
	}

	return policyStmt + ";"
}

// generateAlterPolicySQL generates ALTER POLICY statement for changes that don't require recreation
func generateAlterPolicySQL(old, new *ir.RLSPolicy, targetSchema string) string {
	// Only include table name without schema if it's in the target schema
	tableName := getTableNameWithSchema(new.Schema, new.Table, targetSchema)

	// Check what aspects have changed
	roleChange := !roleListsEqualCaseInsensitive(old.Roles, new.Roles)
	usingChange := old.Using != new.Using
	withCheckChange := old.WithCheck != new.WithCheck

	// Build ALTER POLICY statement with all changes
	alterStmt := fmt.Sprintf("ALTER POLICY %s ON %s", ir.QuoteIdentifier(new.Name), tableName)

	// Add TO clause if roles changed
	if roleChange {
		alterStmt += " TO "
		for i, role := range new.Roles {
			if i > 0 {
				alterStmt += ", "
			}
			alterStmt += role
		}
	}

	// Add USING clause if it changed
	if usingChange {
		alterStmt += fmt.Sprintf(" USING %s", ensureParentheses(new.Using))
	}

	// Add WITH CHECK clause if it changed
	if withCheckChange {
		alterStmt += fmt.Sprintf(" WITH CHECK %s", ensureParentheses(new.WithCheck))
	}

	// If no changes detected, this shouldn't happen
	if !roleChange && !usingChange && !withCheckChange {
		return fmt.Sprintf("-- Policy %s requires recreation", new.Name)
	}

	return alterStmt + ";"
}

// ensureParentheses wraps an expression in parentheses if not already wrapped.
// PostgreSQL's CREATE POLICY and ALTER POLICY syntax requires USING (expr) and WITH CHECK (expr).
// pg_get_expr may return expressions without outer parentheses (e.g., simple function calls).
func ensureParentheses(expr string) string {
	if expr == "" {
		return expr
	}
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "(") && strings.HasSuffix(expr, ")") {
		return expr
	}
	return "(" + expr + ")"
}

// roleListsEqualCaseInsensitive compares two role lists for equality (case-insensitive)
// PostgreSQL role names are case-insensitive by default
func roleListsEqualCaseInsensitive(oldRoles, newRoles []string) bool {
	if len(oldRoles) != len(newRoles) {
		return false
	}
	for i, role := range oldRoles {
		if !strings.EqualFold(newRoles[i], role) {
			return false
		}
	}
	return true
}

// policiesEqual compares two policies for equality
func policiesEqual(old, new *ir.RLSPolicy) bool {
	if old.Schema != new.Schema {
		return false
	}
	if old.Table != new.Table {
		return false
	}
	if old.Name != new.Name {
		return false
	}
	if old.Command != new.Command {
		return false
	}
	if old.Permissive != new.Permissive {
		return false
	}
	if old.Using != new.Using {
		return false
	}
	if old.WithCheck != new.WithCheck {
		return false
	}
	if len(old.Roles) != len(new.Roles) {
		return false
	}
	for i, role := range old.Roles {
		if new.Roles[i] != role {
			return false
		}
	}
	return true
}

// needsRecreate determines if a policy change requires DROP/CREATE instead of ALTER
func needsRecreate(old, new *ir.RLSPolicy) bool {
	// Name changes require recreation (we don't use ALTER POLICY RENAME)
	if old.Name != new.Name {
		return true
	}
	// Command changes require recreation
	if old.Command != new.Command {
		return true
	}
	// Permissive/Restrictive changes require recreation
	if old.Permissive != new.Permissive {
		return true
	}
	// All other changes (roles, using, with_check) can use ALTER POLICY
	return false
}
