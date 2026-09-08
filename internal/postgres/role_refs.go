package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pgplex/pgschema/cmd/util"
)

// pseudoRoles are grantee keywords that never name a real role.
var pseudoRoles = map[string]bool{
	"public":       true,
	"current_user": true,
	"current_role": true,
	"session_user": true,
}

// ExtractReferencedRoles returns the distinct role names the SQL needs to
// exist: grantees of GRANT ... TO and REVOKE ... FROM, roles named by
// CREATE/ALTER POLICY ... TO, and grantors of ALTER DEFAULT PRIVILEGES FOR ROLE.
// Pseudo-roles (PUBLIC, CURRENT_USER, CURRENT_ROLE, SESSION_USER) and the
// predefined pg_* roles are excluded. String literals, comments, and
// dollar-quoted bodies are skipped, so a role created inside a DO block is
// invisible here — only its later use is (issue #450).
func ExtractReferencedRoles(sql string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(role string) {
		if role == "" || pseudoRoles[role] || strings.HasPrefix(role, "pg_") || seen[role] {
			return
		}
		seen[role] = true
		out = append(out, role)
	}

	for _, role := range ExtractDefaultPrivilegeRoles(sql) {
		add(role)
	}

	// Each clause pairs a statement keyword with the keyword that introduces
	// its role list. Searching within a single statement (split on ';') keeps
	// one GRANT's TO from being matched to the next statement's, and lets
	// REVOKE GRANT OPTION FOR ... FROM find no TO after its GRANT and fall
	// through to the REVOKE rule.
	clauses := []struct{ stmt, list string }{
		{"grant", "to"},
		{"revoke", "from"},
		{"policy", "to"},
	}
	for stmt := range strings.SplitSeq(codeOnly(sql), ";") {
		for _, c := range clauses {
			at := indexKeyword(stmt, 0, c.stmt)
			if at < 0 {
				continue
			}
			at = indexKeyword(stmt, at+len(c.stmt), c.list)
			if at < 0 {
				continue
			}
			for _, role := range parseRoleList(stmt, at+len(c.list)) {
				add(role)
			}
		}
	}
	return out
}

// codeOnly returns sql with string literals, comments, and dollar-quoted
// bodies replaced by a space, so a statement stays contiguous even when a
// comment sits between its keywords (GRANT ... TO /* why */ app_user).
func codeOnly(sql string) string {
	var b strings.Builder
	walkSQLCode(sql, func(code string) {
		b.WriteString(code)
		b.WriteByte(' ')
	})
	return b.String()
}

// parseRoleList parses a comma-separated list of role specifications starting
// at i, e.g. `app_user, "Reader", GROUP admins WITH GRANT OPTION` yields the
// three names. It stops at the first token that does not continue the list.
func parseRoleList(s string, i int) []string {
	var roles []string
	for {
		i = skipSpace(s, i)
		if hasKeywordAt(s, i, "group") {
			i = skipSpace(s, i+len("group"))
		}
		role, next, ok := parseIdent(s, i)
		if !ok {
			return roles
		}
		roles = append(roles, role)
		i = skipSpace(s, next)
		if i >= len(s) || s[i] != ',' {
			return roles
		}
		i++
	}
}

// createRoleIfMissing creates a stub role and reports whether this call created
// it. A successful CREATE ROLE is the only proof of ownership: duplicate_object
// means the role pre-existed and is not ours to drop. Without CREATEROLE the
// create is refused before the existence check, so an existing role is then
// confirmed via pg_roles, while a missing one cannot be stubbed at all.
func createRoleIfMissing(ctx context.Context, conn *sql.Conn, role string) (bool, error) {
	_, err := util.ExecContextWithLogging(ctx, conn, "CREATE ROLE "+quoteIdent(role), "create stub role")
	if err == nil {
		return true, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "42710": // duplicate_object
			return false, nil
		case "42501": // insufficient_privilege
			var exists bool
			if qerr := conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)", role).Scan(&exists); qerr == nil && exists {
				return false, nil
			}
			return false, fmt.Errorf("failed to create stub role %q: %w\nHint: the plan database user needs CREATEROLE to stub roles referenced by the schema; grant it, or create the role in the plan database first", role, err)
		}
	}
	return false, fmt.Errorf("failed to create stub role %q: %w", role, err)
}
