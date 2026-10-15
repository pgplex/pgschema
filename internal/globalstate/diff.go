package globalstate

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/pgplex/pgschema/ir"
)

type Change struct {
	SQL              string
	Type             string
	Operation        string
	Path             string
	Phase            string
	RoleName         string
	Membership       *MembershipRef
	Ownership        *Ownership
	DefaultPrivilege *DefaultPrivilege
}

func PlanChanges(manifest Manifest, current Snapshot, majorVersion int) ([]Change, error) {
	if err := validateManifest(manifest); err != nil {
		return nil, err
	}
	roles := append([]Role(nil), manifest.Roles...)
	for i := range roles {
		if roles[i].ValidUntil == "" {
			roles[i].ValidUntil = "infinity"
		}
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	memberships := append([]Membership(nil), manifest.Memberships...)
	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].Role == memberships[j].Role {
			return memberships[i].Member < memberships[j].Member
		}
		return memberships[i].Role < memberships[j].Role
	})

	var changes []Change
	var finalMembershipChanges []Change
	var finalRoleChanges []Change
	available := make(map[string]bool, len(current.Roles)+len(roles))
	createdRoles := make(map[string]bool)
	for name := range current.Roles {
		available[name] = true
	}
	for _, role := range roles {
		actual, exists := current.Roles[role.Name]
		if role.State == StateAbsent {
			if !exists {
				continue
			}
			if role.Name == current.SessionRole.Name || strings.HasPrefix(role.Name, "pg_") {
				return nil, fmt.Errorf("refusing to drop protected role %q", role.Name)
			}
			if current.RoleDependencyCounts[role.Name] > plannedOwnershipReleases(manifest, current, role.Name) {
				return nil, fmt.Errorf("role %q still owns objects or holds privileges outside the managed selection", role.Name)
			}
			finalRoleChanges = append(finalRoleChanges, Change{
				SQL: "DROP ROLE " + ir.QuoteIdentifier(role.Name), Type: "role", Operation: "drop",
				Path: role.Name, Phase: "final", RoleName: role.Name,
			})
			continue
		}
		if role.State == StateExternal {
			if !exists {
				return nil, fmt.Errorf("external role %q does not exist", role.Name)
			}
			continue
		}
		available[role.Name] = true
		operation := "alter"
		if !exists {
			operation = "create"
			createdRoles[role.Name] = true
			changes = append(changes, Change{
				SQL:       roleChangeSQL("CREATE ROLE", role, false),
				Type:      "role",
				Operation: operation,
				Path:      role.Name,
				RoleName:  role.Name,
			})
			continue
		}
		if roleDiffers(role, actual) {
			change := Change{
				SQL:       roleChangeSQL("ALTER ROLE", role, true),
				Type:      "role",
				Operation: operation,
				Path:      role.Name,
				RoleName:  role.Name,
			}
			if roleReducesAuthority(role, actual) {
				change.Phase = "final"
				finalRoleChanges = append(finalRoleChanges, change)
			} else {
				changes = append(changes, change)
			}
		}
	}

	for _, membership := range memberships {
		if !available[membership.Role] {
			return nil, fmt.Errorf("membership role %q is neither present nor declared", membership.Role)
		}
		if !available[membership.Member] {
			return nil, fmt.Errorf("membership member %q is neither present nor declared", membership.Member)
		}
		if majorVersion < 16 && (!membership.Inherit || !membership.Set) {
			return nil, fmt.Errorf("membership INHERIT and SET options require PostgreSQL 16 or newer")
		}
		key := MembershipKey(membership.Role, membership.Member)
		actual, exists := current.Memberships[key]
		if membership.State == StateAbsent {
			if exists {
				change := Change{
					SQL:        fmt.Sprintf("REVOKE %s FROM %s", ir.QuoteIdentifier(membership.Role), ir.QuoteIdentifier(membership.Member)),
					Type:       "role_membership",
					Operation:  "drop",
					Path:       key.Path(),
					Membership: membershipRef(membership),
				}
				if majorVersion >= 16 && len(actual.Grantors) > 1 {
					return nil, fmt.Errorf("membership %q has multiple grantors and cannot be safely revoked", key.Path())
				}
				if majorVersion >= 16 {
					if grantor, foreign := foreignMembershipGrantor(actual, current.SessionRole.Name); foreign {
						return nil, fmt.Errorf("membership %q is owned by foreign grantor %q and cannot be safely revoked", key.Path(), grantor)
					}
				}
				change.Phase = "final"
				finalMembershipChanges = append(finalMembershipChanges, change)
			}
			continue
		}
		omitAdmin := membership.Member == current.SessionRole.Name && membership.Admin &&
			(current.SessionAdminRoles[membership.Role] ||
				(majorVersion >= 16 && createdRoles[membership.Role] && !current.SessionRole.Superuser))
		if membership.Member == current.SessionRole.Name && !membership.Admin &&
			(current.SessionAdminRoles[membership.Role] ||
				(majorVersion >= 16 && createdRoles[membership.Role] && !current.SessionRole.Superuser)) {
			return nil, fmt.Errorf("cannot converge ADMIN false for session role %q on role %q because PostgreSQL owns the creator ADMIN grant", membership.Member, membership.Role)
		}
		if !exists {
			changes = append(changes, Change{
				SQL:        grantMembershipSQL(membership, majorVersion, omitAdmin),
				Type:       "role_membership",
				Operation:  "create",
				Path:       key.Path(),
				Membership: membershipRef(membership),
			})
			continue
		}
		if membership.Admin == actual.Admin && membership.Inherit == actual.Inherit && membership.Set == actual.Set {
			continue
		}
		if majorVersion >= 16 && len(actual.Grantors) > 1 {
			return nil, fmt.Errorf("membership %q has multiple grantors and cannot be safely altered", key.Path())
		}
		if majorVersion >= 16 && membershipReducesAuthority(membership, actual, majorVersion) {
			if grantor, foreign := foreignMembershipGrantor(actual, current.SessionRole.Name); foreign {
				return nil, fmt.Errorf("membership %q is owned by foreign grantor %q and cannot be safely altered", key.Path(), grantor)
			}
		}
		if majorVersion < 16 && actual.Admin && !membership.Admin {
			change := Change{
				SQL:        fmt.Sprintf("REVOKE ADMIN OPTION FOR %s FROM %s", ir.QuoteIdentifier(membership.Role), ir.QuoteIdentifier(membership.Member)),
				Type:       "role_membership",
				Operation:  "alter",
				Path:       key.Path(),
				Membership: membershipRef(membership),
			}
			change.Phase = "final"
			finalMembershipChanges = append(finalMembershipChanges, change)
			continue
		}
		change := Change{
			SQL:        grantMembershipSQL(membership, majorVersion, omitAdmin),
			Type:       "role_membership",
			Operation:  "alter",
			Path:       key.Path(),
			Membership: membershipRef(membership),
		}
		if membershipReducesAuthority(membership, actual, majorVersion) {
			change.Phase = "final"
			finalMembershipChanges = append(finalMembershipChanges, change)
		} else {
			changes = append(changes, change)
		}
	}
	for _, object := range manifest.Ownership {
		if !available[object.Owner] {
			return nil, fmt.Errorf("ownership role %q is neither present nor declared", object.Owner)
		}
		key := ownershipKey(object.Kind, object.Name)
		actual, exists := current.Ownership[key]
		if exists && actual.ExtensionOwned {
			return nil, fmt.Errorf("refusing to manage ownership of extension member %s %q", object.Kind, object.Name)
		}
		if !exists || actual.Owner != object.Owner {
			sql, err := ownershipSQL(object, actual, exists, current.SessionRole)
			if err != nil {
				return nil, err
			}
			objectCopy := object
			changes = append(changes, Change{SQL: sql, Type: "ownership", Operation: "alter", Path: key.Path(), Phase: "post", Ownership: &objectCopy})
		}
	}
	for _, privilege := range manifest.DefaultPrivileges {
		if privilege.ObjectType == "schemas" && majorVersion < 15 {
			return nil, fmt.Errorf("global default privileges on schemas require PostgreSQL 15 or newer")
		}
		if privilege.ObjectType == "tables" && majorVersion < 17 && slices.Contains(privilege.Privileges, "MAINTAIN") {
			return nil, fmt.Errorf("MAINTAIN default privileges require PostgreSQL 17 or newer")
		}
		if !available[privilege.Owner] || (privilege.Grantee != "PUBLIC" && !available[privilege.Grantee]) {
			return nil, fmt.Errorf("default privilege roles must be present or declared")
		}
		key := defaultPrivilegeKey(privilege.Owner, privilege.ObjectType, privilege.Grantee)
		actual := current.DefaultPrivileges[key]
		if defaultPrivilegeMatches(privilege, actual) {
			continue
		}
		operation := "alter"
		if privilege.State == StateAbsent {
			operation = "drop"
		}
		privilegeCopy := privilege
		changes = append(changes, Change{
			SQL:  defaultPrivilegeSQL(privilege, current.SessionRole.Name, !current.SessionRole.Superuser),
			Type: "global_default_privilege", Operation: operation,
			Path: key.Path(), Phase: "post", DefaultPrivilege: &privilegeCopy,
		})
	}
	sort.SliceStable(finalMembershipChanges, func(i, j int) bool {
		iConsumesAuthority := finalMembershipChanges[i].Membership != nil && finalMembershipChanges[i].Membership.Member == current.SessionRole.Name
		jConsumesAuthority := finalMembershipChanges[j].Membership != nil && finalMembershipChanges[j].Membership.Member == current.SessionRole.Name
		return !iConsumesAuthority && jConsumesAuthority
	})
	changes = append(changes, finalMembershipChanges...)
	changes = append(changes, finalRoleChanges...)
	if err := preflightRoleAuthority(manifest, current, changes, majorVersion); err != nil {
		return nil, err
	}
	return changes, nil
}

func plannedOwnershipReleases(manifest Manifest, current Snapshot, owner string) int {
	releases := 0
	for _, object := range manifest.Ownership {
		actual, exists := current.Ownership[ownershipKey(object.Kind, object.Name)]
		if exists && actual.Owner == owner && object.Owner != owner {
			releases++
		}
	}
	return releases
}

func preflightRoleAuthority(manifest Manifest, current Snapshot, changes []Change, majorVersion int) error {
	if current.SessionRole.Name == "" || current.SessionRole.Superuser {
		return nil
	}
	created := make(map[string]bool)
	plannedSet := make(map[MembershipRef]bool)
	for _, role := range manifest.Roles {
		if role.State == StatePresent {
			if _, exists := current.Roles[role.Name]; !exists {
				created[role.Name] = true
			}
		}
	}
	for _, membership := range manifest.Memberships {
		if membership.State == StatePresent && (majorVersion < 16 || membership.Set) {
			plannedSet[MembershipKey(membership.Role, membership.Member)] = true
		}
	}
	for _, change := range changes {
		switch change.Type {
		case "role":
			if !current.SessionRole.CreateRole {
				return fmt.Errorf("session role %q lacks CREATEROLE required by the planned role changes", current.SessionRole.Name)
			}
			if majorVersion < 16 {
				continue
			}
			authorityRole := change.RoleName
			if change.Operation == "create" {
				continue
			}
			if !created[authorityRole] && !current.SessionAdminRoles[authorityRole] {
				return fmt.Errorf("session role %q lacks ADMIN OPTION on role %q required by the planned %s", current.SessionRole.Name, authorityRole, change.Type)
			}
		case "role_membership":
			authorityRole := change.Membership.Role
			if !created[authorityRole] && !current.SessionAdminRoles[authorityRole] {
				return fmt.Errorf("session role %q lacks ADMIN OPTION on role %q required by the planned %s", current.SessionRole.Name, authorityRole, change.Type)
			}
		case "ownership":
			object := *change.Ownership
			ref := ownershipKey(object.Kind, object.Name)
			actual, exists := current.Ownership[ref]
			executor := current.SessionRole.Name
			if exists && actual.Owner != current.SessionRole.Name {
				executor = actual.Owner
			}
			if exists && actual.Owner != current.SessionRole.Name && !current.SessionSetRoles[actual.Owner] &&
				!plannedSet[MembershipKey(actual.Owner, current.SessionRole.Name)] {
				return fmt.Errorf("session role %q cannot assume current owner role %q for %s", current.SessionRole.Name, actual.Owner, change.Path)
			}
			if exists && actual.Owner != object.Owner && actual.Owner != current.SessionRole.Name &&
				!current.OwnerSetRoles[RoleTransition{From: actual.Owner, To: object.Owner}] &&
				!plannedSet[MembershipKey(object.Owner, actual.Owner)] {
				return fmt.Errorf("current owner role %q lacks SET authority on new owner role %q for %s", actual.Owner, object.Owner, change.Path)
			}
			if object.Owner != current.SessionRole.Name && !current.SessionSetRoles[object.Owner] &&
				!plannedSet[MembershipKey(object.Owner, current.SessionRole.Name)] {
				return fmt.Errorf("session role %q lacks SET authority on new owner role %q for %s; declare a SET-enabled membership", current.SessionRole.Name, object.Owner, change.Path)
			}
			if object.Kind == "database" {
				if exists && actual.Owner != current.SessionRole.Name && !current.CurrentOwnerDatabaseAuthority[ref] {
					return fmt.Errorf("current owner role %q lacks CREATEDB required to change database ownership", actual.Owner)
				}
				if (!exists || actual.Owner == current.SessionRole.Name) && !current.SessionRole.CreateDB {
					return fmt.Errorf("session role %q lacks CREATEDB required to change database ownership", current.SessionRole.Name)
				}
			}
			if object.Kind == "database" && !current.NewOwnerCreatePrivileges[ref] &&
				!plannedNewOwnerCreateAuthority(manifest, current, object) {
				return fmt.Errorf("new owner role %q lacks CREATEDB required to change database ownership", object.Owner)
			}
			if object.Kind == "schema" && !current.OwnershipExecutorCreatePrivileges[ref] {
				return fmt.Errorf("current owner role %q lacks required CREATE authority for %s %q", executor, object.Kind, object.Name)
			}
			if object.Kind != "database" && object.Kind != "schema" &&
				!current.NewOwnerCreatePrivileges[ref] && !plannedNewOwnerCreateAuthority(manifest, current, object) {
				return fmt.Errorf("new owner role %q lacks required CREATE authority for %s %q", object.Owner, object.Kind, object.Name)
			}
		case "global_default_privilege":
			privilege := *change.DefaultPrivilege
			if privilege.Owner != current.SessionRole.Name && !current.SessionSetRoles[privilege.Owner] &&
				!plannedSet[MembershipKey(privilege.Owner, current.SessionRole.Name)] {
				return fmt.Errorf("session role %q lacks SET authority on default-privilege owner role %q; declare a SET-enabled membership", current.SessionRole.Name, privilege.Owner)
			}
		}
	}
	for _, role := range manifest.Roles {
		actual, exists := current.Roles[role.Name]
		if role.State != StatePresent || (!exists && !role.Replication && !role.BypassRLS) {
			continue
		}
		if (!exists && (role.Replication || role.BypassRLS)) || (exists && (actual.Superuser || role.Replication != actual.Replication || role.BypassRLS != actual.BypassRLS)) {
			return fmt.Errorf("session role %q must be superuser to change SUPERUSER, REPLICATION, or BYPASSRLS on role %q", current.SessionRole.Name, role.Name)
		}
	}
	return nil
}

func membershipRef(membership Membership) *MembershipRef {
	return &MembershipRef{Role: membership.Role, Member: membership.Member}
}

func foreignMembershipGrantor(actual MembershipState, sessionRole string) (string, bool) {
	if len(actual.Grantors) != 1 || actual.Grantors[0] == sessionRole {
		return "", false
	}
	return actual.Grantors[0], true
}

func membershipReducesAuthority(desired Membership, actual MembershipState, majorVersion int) bool {
	if actual.Admin && !desired.Admin {
		return true
	}
	return majorVersion >= 16 && ((actual.Inherit && !desired.Inherit) || (actual.Set && !desired.Set))
}

func roleReducesAuthority(desired Role, actual RoleState) bool {
	return actual.Superuser || (actual.CreateRole && !desired.CreateRole) || (actual.CreateDB && !desired.CreateDB) ||
		(actual.Inherit && !desired.Inherit)
}

func ValidateExecutionRole(manifest Manifest, current Snapshot, executionRole string, majorVersion int) error {
	if executionRole == "" || executionRole == current.SessionRole.Name || current.SessionRole.Superuser ||
		current.SessionSetRoles[executionRole] {
		return nil
	}
	for _, membership := range manifest.Memberships {
		if membership.State == StatePresent && membership.Role == executionRole &&
			membership.Member == current.SessionRole.Name && (majorVersion < 16 || membership.Set) {
			return nil
		}
	}
	return fmt.Errorf("session role %q lacks SET authority on schema execution role %q; declare a SET-enabled membership", current.SessionRole.Name, executionRole)
}

func plannedNewOwnerCreateAuthority(manifest Manifest, current Snapshot, object Ownership) bool {
	if object.Kind == "database" {
		for _, role := range manifest.Roles {
			if role.Name == object.Owner && role.State == StatePresent && role.CreateDB {
				return true
			}
		}
		role := current.Roles[object.Owner]
		return role.Superuser || role.CreateDB
	}
	schema := ""
	if object.Kind == "function" || object.Kind == "procedure" {
		open := strings.IndexByte(object.Name, '(')
		if open < 0 {
			return false
		}
		schema, _, _ = splitQualifiedName(object.Name[:open])
	} else {
		schema, _, _ = splitQualifiedName(object.Name)
	}
	return schema != "" && manifestTransfersContainerOwnership(manifest, "schema", schema, object.Owner)
}

func manifestTransfersContainerOwnership(manifest Manifest, kind, name, owner string) bool {
	for _, candidate := range manifest.Ownership {
		if candidate.Kind == kind && candidate.Owner == owner && (name == "" || candidate.Name == name) {
			return true
		}
	}
	return false
}

func roleDiffers(desired Role, actual RoleState) bool {
	return desired.Login != actual.Login || desired.Inherit != actual.Inherit ||
		desired.CreateDB != actual.CreateDB || desired.CreateRole != actual.CreateRole ||
		desired.Replication != actual.Replication || desired.BypassRLS != actual.BypassRLS ||
		actual.Superuser || desired.ConnectionLimit != actual.ConnectionLimit || !validUntilMatches(desired.ValidUntil, actual.ValidUntil) ||
		!maps.Equal(desired.Configuration, actual.Configuration)
}

func roleOptions(role Role) string {
	return strings.Join([]string{
		"NOSUPERUSER",
		boolOption(role.Login, "LOGIN", "NOLOGIN"),
		boolOption(role.Inherit, "INHERIT", "NOINHERIT"),
		boolOption(role.CreateDB, "CREATEDB", "NOCREATEDB"),
		boolOption(role.CreateRole, "CREATEROLE", "NOCREATEROLE"),
		boolOption(role.Replication, "REPLICATION", "NOREPLICATION"),
		boolOption(role.BypassRLS, "BYPASSRLS", "NOBYPASSRLS"),
		fmt.Sprintf("CONNECTION LIMIT %d", role.ConnectionLimit),
		"VALID UNTIL " + quoteLiteral(role.ValidUntil),
	}, " ")
}

func roleChangeSQL(command string, role Role, resetConfiguration bool) string {
	name := ir.QuoteIdentifier(role.Name)
	statements := []string{command + " " + name + " WITH " + roleOptions(role)}
	if resetConfiguration {
		statements = append(statements, "ALTER ROLE "+name+" RESET ALL")
	}
	keys := make([]string, 0, len(role.Configuration))
	for key := range role.Configuration {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		statements = append(statements, "ALTER ROLE "+name+" SET "+key+" TO "+quoteLiteral(role.Configuration[key]))
	}
	return strings.Join(statements, "; ")
}

func validUntilMatches(desired, actual string) bool {
	if desired == "infinity" {
		return actual == "infinity"
	}
	parsed, err := time.Parse(time.RFC3339, desired)
	return err == nil && fmt.Sprintf("%d", parsed.UnixMicro()) == actual
}

func quoteLiteral(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return "E'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func ownershipSQL(object Ownership, actual OwnershipState, exists bool, session RoleState) (string, error) {
	owner := ir.QuoteIdentifier(object.Owner)
	if object.Kind == "database" || object.Kind == "schema" {
		return ownershipAsCurrentOwner(fmt.Sprintf("ALTER %s %s OWNER TO %s", strings.ToUpper(object.Kind), ir.QuoteIdentifier(object.Name), owner), actual, exists, session), nil
	}
	if object.Kind == "function" || object.Kind == "procedure" {
		routine, err := renderRoutineName(object.Name)
		if err != nil {
			return "", err
		}
		return ownershipAsCurrentOwner(fmt.Sprintf("ALTER %s %s OWNER TO %s", strings.ToUpper(object.Kind), routine, owner), actual, exists, session), nil
	}
	schema, name, err := splitQualifiedName(object.Name)
	if err != nil {
		return "", err
	}
	kind := strings.ToUpper(strings.ReplaceAll(object.Kind, "_", " "))
	return ownershipAsCurrentOwner(fmt.Sprintf("ALTER %s %s.%s OWNER TO %s", kind, ir.QuoteIdentifier(schema), ir.QuoteIdentifier(name), owner), actual, exists, session), nil
}

func ownershipAsCurrentOwner(sql string, actual OwnershipState, exists bool, session RoleState) string {
	if !exists || session.Superuser || actual.Owner == session.Name {
		return sql
	}
	return "SET ROLE " + ir.QuoteIdentifier(actual.Owner) + "; " + sql + "; RESET ROLE"
}

func renderRoutineName(name string) (string, error) {
	open := strings.IndexByte(name, '(')
	if open < 0 || !strings.HasSuffix(name, ")") || strings.ContainsAny(name, ";\n\r") || strings.Contains(name, "--") || strings.Contains(name, "/*") {
		return "", fmt.Errorf("routine name %q must be a safe schema-qualified identity signature", name)
	}
	schema, routine, err := splitQualifiedName(name[:open])
	if err != nil {
		return "", err
	}
	return ir.QuoteIdentifier(schema) + "." + ir.QuoteIdentifier(routine) + name[open:], nil
}

func defaultPrivilegeMatches(desired DefaultPrivilege, actual DefaultPrivilegeState) bool {
	if desired.State == StateAbsent {
		return len(actual.Privileges) == 0
	}
	if len(desired.Privileges) != len(actual.Privileges) {
		return false
	}
	for _, privilege := range desired.Privileges {
		if grantable, exists := actual.Privileges[privilege]; !exists || grantable != desired.GrantOption {
			return false
		}
	}
	return true
}

func defaultPrivilegeSQL(privilege DefaultPrivilege, sessionRole string, assumeOwner bool) string {
	base := "ALTER DEFAULT PRIVILEGES FOR ROLE " + ir.QuoteIdentifier(privilege.Owner)
	grantee := privilege.Grantee
	if grantee != "PUBLIC" {
		grantee = ir.QuoteIdentifier(grantee)
	}
	revoke := base + " REVOKE ALL ON " + strings.ToUpper(privilege.ObjectType) + " FROM " + grantee
	if privilege.State == StateAbsent {
		return defaultPrivilegeAsOwner(revoke, privilege.Owner, sessionRole, assumeOwner)
	}
	grant := base + " GRANT " + strings.Join(privilege.Privileges, ", ") + " ON " + strings.ToUpper(privilege.ObjectType) + " TO " + grantee
	if privilege.GrantOption {
		grant += " WITH GRANT OPTION"
	}
	return defaultPrivilegeAsOwner(revoke+"; "+grant, privilege.Owner, sessionRole, assumeOwner)
}

func defaultPrivilegeAsOwner(sql, owner, sessionRole string, assumeOwner bool) string {
	if !assumeOwner || owner == sessionRole || sessionRole == "" {
		return sql
	}
	return "SET ROLE " + ir.QuoteIdentifier(owner) + "; " + sql + "; RESET ROLE"
}

func boolOption(value bool, enabled, disabled string) string {
	if value {
		return enabled
	}
	return disabled
}

func grantMembershipSQL(membership Membership, majorVersion int, omitAdmin bool) string {
	sql := fmt.Sprintf("GRANT %s TO %s", ir.QuoteIdentifier(membership.Role), ir.QuoteIdentifier(membership.Member))
	if majorVersion >= 16 {
		options := make([]string, 0, 3)
		if !omitAdmin {
			options = append(options, "ADMIN "+boolKeyword(membership.Admin))
		}
		options = append(options, "INHERIT "+boolKeyword(membership.Inherit), "SET "+boolKeyword(membership.Set))
		return sql + " WITH " + strings.Join(options, ", ")
	}
	if membership.Admin {
		return sql + " WITH ADMIN OPTION"
	}
	return sql
}

func boolKeyword(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}
