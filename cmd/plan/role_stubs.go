package plan

import (
	"context"
	"fmt"
	"strings"

	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/postgres"
	"github.com/pgplex/pgschema/ir"
)

// validateReferencedRoles rejects desired-state SQL that grants to roles the
// target database does not have unless the opt-in global manifest declares
// that role as present. The plan provider stubs validated references in its
// throwaway database so desired schema SQL can be inspected (issue #450).
func validateReferencedRoles(ctx context.Context, cfg *util.ConnectionConfig, desiredSQL string, plannedRoles map[string]bool) error {
	referenced := postgres.ExtractReferencedRoles(desiredSQL)
	if len(referenced) == 0 {
		return nil
	}

	conn, err := util.Connect(cfg)
	if err != nil {
		return fmt.Errorf("connect to check referenced roles: %w", err)
	}
	defer conn.Close()

	rows, err := conn.QueryContext(ctx, "SELECT rolname FROM pg_catalog.pg_roles")
	if err != nil {
		return fmt.Errorf("query target roles: %w", err)
	}
	defer rows.Close()
	existing := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan target role: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read target roles: %w", err)
	}

	var missing []string
	for _, role := range referenced {
		if !existing[role] && !plannedRoles[role] {
			missing = append(missing, ir.QuoteIdentifier(role))
		}
	}
	switch len(missing) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("role %s is referenced by the schema but does not exist on the target database; create it first or declare it in --global-file, see https://www.pgschema.com/cli/plan-db", missing[0])
	default:
		return fmt.Errorf("roles %s are referenced by the schema but do not exist on the target database; create them first or declare them in --global-file, see https://www.pgschema.com/cli/plan-db", strings.Join(missing, ", "))
	}
}
