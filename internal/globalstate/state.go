package globalstate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"
)

type RoleState struct {
	Name            string            `json:"name"`
	Superuser       bool              `json:"superuser"`
	Login           bool              `json:"login"`
	Inherit         bool              `json:"inherit"`
	CreateDB        bool              `json:"createdb"`
	CreateRole      bool              `json:"createrole"`
	Replication     bool              `json:"replication"`
	BypassRLS       bool              `json:"bypassrls"`
	ConnectionLimit int               `json:"connection_limit"`
	ValidUntil      string            `json:"valid_until"`
	Configuration   map[string]string `json:"configuration,omitempty"`
}

type MembershipState struct {
	Role     string   `json:"role"`
	Member   string   `json:"member"`
	Admin    bool     `json:"admin"`
	Inherit  bool     `json:"inherit"`
	Set      bool     `json:"set"`
	Grantors []string `json:"grantors,omitempty"`
}

type Snapshot struct {
	Roles                             map[string]RoleState
	Memberships                       map[MembershipRef]MembershipState
	Ownership                         map[OwnershipRef]OwnershipState
	DefaultPrivileges                 map[DefaultPrivilegeRef]DefaultPrivilegeState
	SessionRole                       RoleState
	SessionAdminRoles                 map[string]bool
	SessionSetRoles                   map[string]bool
	OwnerSetRoles                     map[RoleTransition]bool
	NewOwnerCreatePrivileges          map[OwnershipRef]bool
	OwnershipExecutorCreatePrivileges map[OwnershipRef]bool
	CurrentOwnerDatabaseAuthority     map[OwnershipRef]bool
	RoleDependencyCounts              map[string]int
	DatabaseName                      string
}

type OwnershipState struct {
	Kind           string `json:"kind"`
	Name           string `json:"name"`
	Owner          string `json:"owner"`
	ExtensionOwned bool   `json:"extension_owned,omitempty"`
}

type RoleTransition struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DefaultPrivilegeState struct {
	Owner      string          `json:"owner"`
	ObjectType string          `json:"object_type"`
	Grantee    string          `json:"grantee"`
	Privileges map[string]bool `json:"privileges"`
}

type MembershipRef struct {
	Role   string `json:"role"`
	Member string `json:"member"`
}

type OwnershipRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type Selection struct {
	Roles             []string              `json:"roles"`
	Memberships       []MembershipRef       `json:"memberships"`
	Ownership         []Ownership           `json:"ownership"`
	DefaultPrivileges []DefaultPrivilegeRef `json:"default_privileges"`
}

type DefaultPrivilegeRef struct {
	Owner      string `json:"owner"`
	ObjectType string `json:"object_type"`
	Grantee    string `json:"grantee"`
}

type Fingerprint struct {
	Hash      string    `json:"hash"`
	Selection Selection `json:"selection"`
}

func SelectionFor(manifest Manifest) Selection {
	roleNames := make(map[string]struct{})
	for _, role := range manifest.Roles {
		roleNames[role.Name] = struct{}{}
	}
	selection := Selection{}
	for _, membership := range manifest.Memberships {
		roleNames[membership.Role] = struct{}{}
		roleNames[membership.Member] = struct{}{}
		selection.Memberships = append(selection.Memberships, MembershipRef{Role: membership.Role, Member: membership.Member})
	}
	for _, object := range manifest.Ownership {
		roleNames[object.Owner] = struct{}{}
		selection.Ownership = append(selection.Ownership, object)
	}
	for _, privilege := range manifest.DefaultPrivileges {
		roleNames[privilege.Owner] = struct{}{}
		if privilege.Grantee != "PUBLIC" {
			roleNames[privilege.Grantee] = struct{}{}
		}
		selection.DefaultPrivileges = append(selection.DefaultPrivileges, DefaultPrivilegeRef{
			Owner: privilege.Owner, ObjectType: privilege.ObjectType, Grantee: privilege.Grantee,
		})
	}
	for role := range roleNames {
		selection.Roles = append(selection.Roles, role)
	}
	normalizeSelection(&selection)
	return selection
}

func Inspect(ctx context.Context, db *sql.DB, selection Selection, majorVersion int) (Snapshot, error) {
	normalizeSelection(&selection)
	snapshot := Snapshot{
		Roles:                             make(map[string]RoleState),
		Memberships:                       make(map[MembershipRef]MembershipState),
		Ownership:                         make(map[OwnershipRef]OwnershipState),
		DefaultPrivileges:                 make(map[DefaultPrivilegeRef]DefaultPrivilegeState),
		SessionAdminRoles:                 make(map[string]bool),
		SessionSetRoles:                   make(map[string]bool),
		OwnerSetRoles:                     make(map[RoleTransition]bool),
		NewOwnerCreatePrivileges:          make(map[OwnershipRef]bool),
		OwnershipExecutorCreatePrivileges: make(map[OwnershipRef]bool),
		CurrentOwnerDatabaseAuthority:     make(map[OwnershipRef]bool),
		RoleDependencyCounts:              make(map[string]int),
	}
	var sessionConfig pq.StringArray
	if err := db.QueryRowContext(ctx, `
SELECT rolname, rolsuper, rolcanlogin, rolinherit, rolcreatedb, rolcreaterole,
       rolreplication, rolbypassrls, rolconnlimit,
       CASE WHEN rolvaliduntil IS NULL OR rolvaliduntil = 'infinity'::timestamptz THEN 'infinity'
            ELSE ((extract(epoch FROM rolvaliduntil) * 1000000)::bigint)::text END,
       COALESCE(rolconfig, '{}'::text[])
FROM pg_catalog.pg_roles WHERE rolname = current_user`).Scan(
		&snapshot.SessionRole.Name, &snapshot.SessionRole.Superuser, &snapshot.SessionRole.Login,
		&snapshot.SessionRole.Inherit, &snapshot.SessionRole.CreateDB, &snapshot.SessionRole.CreateRole,
		&snapshot.SessionRole.Replication, &snapshot.SessionRole.BypassRLS,
		&snapshot.SessionRole.ConnectionLimit, &snapshot.SessionRole.ValidUntil,
		&sessionConfig,
	); err != nil {
		return Snapshot{}, fmt.Errorf("inspect session role: %w", err)
	}
	snapshot.SessionRole.Configuration = parseRoleConfiguration(sessionConfig)
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&snapshot.DatabaseName); err != nil {
		return Snapshot{}, fmt.Errorf("inspect current database: %w", err)
	}
	adminRows, err := db.QueryContext(ctx, `
SELECT granted.rolname
FROM pg_catalog.pg_auth_members m
JOIN pg_catalog.pg_roles granted ON granted.oid = m.roleid
JOIN pg_catalog.pg_roles member ON member.oid = m.member
WHERE member.rolname = current_user AND m.admin_option`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect session role authority: %w", err)
	}
	for adminRows.Next() {
		var role string
		if err := adminRows.Scan(&role); err != nil {
			adminRows.Close()
			return Snapshot{}, fmt.Errorf("scan session role authority: %w", err)
		}
		snapshot.SessionAdminRoles[role] = true
	}
	if err := adminRows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("inspect session role authority: %w", err)
	}
	setPrivilege := "MEMBER"
	if majorVersion >= 16 {
		setPrivilege = "SET"
	}
	setRows, err := db.QueryContext(ctx, `SELECT rolname, pg_has_role(current_user, oid, $1) FROM pg_catalog.pg_roles`, setPrivilege)
	if err != nil {
		return Snapshot{}, fmt.Errorf("inspect session SET authority: %w", err)
	}
	for setRows.Next() {
		var role string
		var canSet bool
		if err := setRows.Scan(&role, &canSet); err != nil {
			setRows.Close()
			return Snapshot{}, fmt.Errorf("scan session SET authority: %w", err)
		}
		if canSet {
			snapshot.SessionSetRoles[role] = true
		}
	}
	if err := setRows.Close(); err != nil {
		return Snapshot{}, fmt.Errorf("inspect session SET authority: %w", err)
	}
	if len(selection.Roles) > 0 {
		rows, err := db.QueryContext(ctx, `
SELECT rolname, rolsuper, rolcanlogin, rolinherit, rolcreatedb, rolcreaterole,
       rolreplication, rolbypassrls, rolconnlimit,
       CASE WHEN rolvaliduntil IS NULL OR rolvaliduntil = 'infinity'::timestamptz THEN 'infinity'
            ELSE ((extract(epoch FROM rolvaliduntil) * 1000000)::bigint)::text END,
       COALESCE(rolconfig, '{}'::text[])
FROM pg_catalog.pg_roles
WHERE rolname = ANY($1)
ORDER BY rolname`, pq.Array(selection.Roles))
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect global roles: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var role RoleState
			var configEntries pq.StringArray
			if err := rows.Scan(&role.Name, &role.Superuser, &role.Login, &role.Inherit, &role.CreateDB, &role.CreateRole, &role.Replication, &role.BypassRLS, &role.ConnectionLimit, &role.ValidUntil, &configEntries); err != nil {
				return Snapshot{}, fmt.Errorf("scan global role: %w", err)
			}
			role.Configuration = parseRoleConfiguration(configEntries)
			snapshot.Roles[role.Name] = role
		}
		if err := rows.Err(); err != nil {
			return Snapshot{}, fmt.Errorf("inspect global roles: %w", err)
		}
		dependencyRows, err := db.QueryContext(ctx, `
SELECT r.rolname, (
  SELECT count(*) FROM pg_catalog.pg_shdepend d
  WHERE d.refclassid = 'pg_authid'::regclass AND d.refobjid = r.oid
    AND d.deptype <> 'p'
)
FROM pg_catalog.pg_roles r
WHERE r.rolname = ANY($1)`, pq.Array(selection.Roles))
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect global role dependencies: %w", err)
		}
		for dependencyRows.Next() {
			var role string
			var count int
			if err := dependencyRows.Scan(&role, &count); err != nil {
				dependencyRows.Close()
				return Snapshot{}, fmt.Errorf("scan global role dependencies: %w", err)
			}
			snapshot.RoleDependencyCounts[role] = count
		}
		if err := dependencyRows.Close(); err != nil {
			return Snapshot{}, fmt.Errorf("inspect global role dependencies: %w", err)
		}
	}

	if len(selection.Memberships) > 0 {
		options := "bool_or(m.admin_option), true AS inherit_option, true AS set_option, ARRAY[pg_get_userbyid(min(m.grantor))]"
		if majorVersion >= 16 {
			options = "bool_or(m.admin_option), bool_or(m.inherit_option), bool_or(m.set_option), array_agg(DISTINCT pg_get_userbyid(m.grantor) ORDER BY pg_get_userbyid(m.grantor))"
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(`
SELECT granted.rolname, member.rolname, %s
FROM pg_catalog.pg_auth_members m
JOIN pg_catalog.pg_roles granted ON granted.oid = m.roleid
JOIN pg_catalog.pg_roles member ON member.oid = m.member
GROUP BY granted.rolname, member.rolname
ORDER BY granted.rolname, member.rolname`, options))
		if err != nil {
			return Snapshot{}, fmt.Errorf("inspect role memberships: %w", err)
		}
		defer rows.Close()
		wanted := make(map[MembershipRef]struct{}, len(selection.Memberships))
		for _, membership := range selection.Memberships {
			wanted[MembershipKey(membership.Role, membership.Member)] = struct{}{}
		}
		for rows.Next() {
			var membership MembershipState
			var grantors pq.StringArray
			if err := rows.Scan(&membership.Role, &membership.Member, &membership.Admin, &membership.Inherit, &membership.Set, &grantors); err != nil {
				return Snapshot{}, fmt.Errorf("scan role membership: %w", err)
			}
			membership.Grantors = append([]string(nil), grantors...)
			key := MembershipKey(membership.Role, membership.Member)
			if _, ok := wanted[key]; ok {
				snapshot.Memberships[key] = membership
			}
		}
		if err := rows.Err(); err != nil {
			return Snapshot{}, fmt.Errorf("inspect role memberships: %w", err)
		}
	}
	for _, object := range selection.Ownership {
		ref := ownershipKey(object.Kind, object.Name)
		state, exists, err := inspectOwnership(ctx, db, object)
		if err != nil {
			return Snapshot{}, err
		}
		if object.Kind != "schema" {
			canCreate, err := inspectRoleCreatePrivilege(ctx, db, object, object.Owner)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.NewOwnerCreatePrivileges[ref] = canCreate
		} else {
			executor := snapshot.SessionRole.Name
			if exists && !snapshot.SessionRole.Superuser && state.Owner != snapshot.SessionRole.Name {
				executor = state.Owner
			}
			canCreate, err := inspectRoleCreatePrivilege(ctx, db, object, executor)
			if err != nil {
				return Snapshot{}, err
			}
			snapshot.OwnershipExecutorCreatePrivileges[ref] = canCreate
		}
		if exists {
			snapshot.Ownership[ref] = state
			if object.Kind == "database" {
				canAlterDatabase, err := inspectRoleDatabaseAuthority(ctx, db, state.Owner)
				if err != nil {
					return Snapshot{}, err
				}
				snapshot.CurrentOwnerDatabaseAuthority[ref] = canAlterDatabase
			}
			transition := RoleTransition{From: state.Owner, To: object.Owner}
			_, desiredOwnerExists := snapshot.Roles[object.Owner]
			if desiredOwnerExists && !snapshot.SessionRole.Superuser && state.Owner != snapshot.SessionRole.Name {
				if _, inspected := snapshot.OwnerSetRoles[transition]; inspected {
					continue
				}
				var canSet bool
				if err := db.QueryRowContext(ctx,
					"SELECT pg_has_role($1, $2, $3)", transition.From, transition.To, setPrivilege,
				).Scan(&canSet); err != nil {
					return Snapshot{}, fmt.Errorf("inspect ownership SET authority from %q to %q: %w", transition.From, transition.To, err)
				}
				snapshot.OwnerSetRoles[transition] = canSet
			}
		}
	}
	if len(selection.DefaultPrivileges) > 0 {
		if err := inspectDefaultPrivileges(ctx, db, selection.DefaultPrivileges, snapshot.DefaultPrivileges); err != nil {
			return Snapshot{}, err
		}
		for _, ref := range selection.DefaultPrivileges {
			if _, exists := snapshot.Roles[ref.Owner]; exists || ref.Grantee != "PUBLIC" {
				continue
			}
			implicit := implicitPublicDefaultPrivileges(ref.ObjectType)
			if len(implicit) > 0 {
				snapshot.DefaultPrivileges[defaultPrivilegeKey(ref.Owner, ref.ObjectType, ref.Grantee)] = DefaultPrivilegeState{
					Owner: ref.Owner, ObjectType: ref.ObjectType, Grantee: ref.Grantee, Privileges: implicit,
				}
			}
		}
	}
	return snapshot, nil
}

func implicitPublicDefaultPrivileges(objectType string) map[string]bool {
	switch objectType {
	case "functions":
		return map[string]bool{"EXECUTE": false}
	case "types":
		return map[string]bool{"USAGE": false}
	default:
		return nil
	}
}

func ComputeFingerprint(snapshot Snapshot, selection Selection) (*Fingerprint, error) {
	normalizeSelection(&selection)
	type fingerprintState struct {
		Roles             []*RoleState             `json:"roles"`
		Memberships       []*MembershipState       `json:"memberships"`
		Ownership         []*OwnershipState        `json:"ownership"`
		DefaultPrivileges []*DefaultPrivilegeState `json:"default_privileges"`
		Authority         authorityState           `json:"authority"`
		OwnerTransitions  []ownerTransitionState   `json:"owner_transitions,omitempty"`
		OwnerCreate       []bool                   `json:"owner_create"`
		ExecutorCreate    []bool                   `json:"executor_create"`
		OwnerDatabase     []bool                   `json:"owner_database"`
		RoleDependencies  []int                    `json:"role_dependencies"`
	}
	authority := authorityState{
		Name: snapshot.SessionRole.Name, Superuser: snapshot.SessionRole.Superuser,
		CreateRole: snapshot.SessionRole.CreateRole, CreateDB: snapshot.SessionRole.CreateDB,
		Database: snapshot.DatabaseName,
	}
	relevantAuthorityRoles := make(map[string]struct{}, len(selection.Roles)+len(selection.Ownership))
	for _, name := range selection.Roles {
		relevantAuthorityRoles[name] = struct{}{}
	}
	for _, object := range selection.Ownership {
		if actual, exists := snapshot.Ownership[ownershipKey(object.Kind, object.Name)]; exists {
			relevantAuthorityRoles[actual.Owner] = struct{}{}
		}
	}
	for name := range snapshot.SessionAdminRoles {
		if _, relevant := relevantAuthorityRoles[name]; relevant {
			authority.AdminRoles = append(authority.AdminRoles, name)
		}
	}
	for name := range snapshot.SessionSetRoles {
		if _, relevant := relevantAuthorityRoles[name]; relevant {
			authority.SetRoles = append(authority.SetRoles, name)
		}
	}
	sort.Strings(authority.AdminRoles)
	sort.Strings(authority.SetRoles)
	state := fingerprintState{Authority: authority}
	for transition, canSet := range snapshot.OwnerSetRoles {
		if canSet {
			state.OwnerTransitions = append(state.OwnerTransitions, ownerTransitionState{From: transition.From, To: transition.To})
		}
	}
	sort.Slice(state.OwnerTransitions, func(i, j int) bool {
		return encodePath(state.OwnerTransitions[i].From, state.OwnerTransitions[i].To) <
			encodePath(state.OwnerTransitions[j].From, state.OwnerTransitions[j].To)
	})
	for _, object := range selection.Ownership {
		ref := ownershipKey(object.Kind, object.Name)
		state.OwnerCreate = append(state.OwnerCreate, snapshot.NewOwnerCreatePrivileges[ref])
		state.ExecutorCreate = append(state.ExecutorCreate, snapshot.OwnershipExecutorCreatePrivileges[ref])
		state.OwnerDatabase = append(state.OwnerDatabase, snapshot.CurrentOwnerDatabaseAuthority[ref])
		if stateValue, exists := snapshot.Ownership[ref]; exists {
			copy := stateValue
			state.Ownership = append(state.Ownership, &copy)
		} else {
			state.Ownership = append(state.Ownership, nil)
		}
	}
	for _, ref := range selection.DefaultPrivileges {
		if stateValue, exists := snapshot.DefaultPrivileges[defaultPrivilegeKey(ref.Owner, ref.ObjectType, ref.Grantee)]; exists {
			copy := stateValue
			state.DefaultPrivileges = append(state.DefaultPrivileges, &copy)
		} else {
			state.DefaultPrivileges = append(state.DefaultPrivileges, nil)
		}
	}
	for _, name := range selection.Roles {
		state.RoleDependencies = append(state.RoleDependencies, snapshot.RoleDependencyCounts[name])
		if role, exists := snapshot.Roles[name]; exists {
			roleCopy := role
			state.Roles = append(state.Roles, &roleCopy)
		} else {
			state.Roles = append(state.Roles, nil)
		}
	}
	for _, ref := range selection.Memberships {
		if membership, exists := snapshot.Memberships[MembershipKey(ref.Role, ref.Member)]; exists {
			membershipCopy := membership
			state.Memberships = append(state.Memberships, &membershipCopy)
		} else {
			state.Memberships = append(state.Memberships, nil)
		}
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode global state fingerprint: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return &Fingerprint{Hash: fmt.Sprintf("%x", hash), Selection: selection}, nil
}

type authorityState struct {
	Name       string   `json:"name"`
	Superuser  bool     `json:"superuser"`
	CreateRole bool     `json:"createrole"`
	CreateDB   bool     `json:"createdb"`
	AdminRoles []string `json:"admin_roles,omitempty"`
	SetRoles   []string `json:"set_roles,omitempty"`
	Database   string   `json:"database"`
}

type ownerTransitionState struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func normalizeSelection(selection *Selection) {
	sort.Strings(selection.Roles)
	sort.Slice(selection.Memberships, func(i, j int) bool {
		if selection.Memberships[i].Role == selection.Memberships[j].Role {
			return selection.Memberships[i].Member < selection.Memberships[j].Member
		}
		return selection.Memberships[i].Role < selection.Memberships[j].Role
	})
	sort.Slice(selection.Ownership, func(i, j int) bool {
		return ownershipKey(selection.Ownership[i].Kind, selection.Ownership[i].Name).Path() < ownershipKey(selection.Ownership[j].Kind, selection.Ownership[j].Name).Path()
	})
	sort.Slice(selection.DefaultPrivileges, func(i, j int) bool {
		return defaultPrivilegeKey(selection.DefaultPrivileges[i].Owner, selection.DefaultPrivileges[i].ObjectType, selection.DefaultPrivileges[i].Grantee).Path() < defaultPrivilegeKey(selection.DefaultPrivileges[j].Owner, selection.DefaultPrivileges[j].ObjectType, selection.DefaultPrivileges[j].Grantee).Path()
	})
}

func MembershipKey(role, member string) MembershipRef {
	return MembershipRef{Role: role, Member: member}
}

func ownershipKey(kind, name string) OwnershipRef {
	return OwnershipRef{Kind: kind, Name: name}
}

func defaultPrivilegeKey(owner, objectType, grantee string) DefaultPrivilegeRef {
	return DefaultPrivilegeRef{Owner: owner, ObjectType: objectType, Grantee: grantee}
}

func encodePath(parts ...string) string {
	var path strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&path, "%d:%s", len(part), part)
	}
	return path.String()
}

func (ref MembershipRef) Path() string {
	return encodePath(ref.Role, ref.Member)
}

func (ref OwnershipRef) Path() string {
	return encodePath(ref.Kind, ref.Name)
}

func (ref DefaultPrivilegeRef) Path() string {
	return encodePath(ref.Owner, ref.ObjectType, ref.Grantee)
}

func parseRoleConfiguration(entries []string) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	configuration := make(map[string]string, len(entries))
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			configuration[name] = value
		}
	}
	return configuration
}

func inspectOwnership(ctx context.Context, db *sql.DB, object Ownership) (OwnershipState, bool, error) {
	query, args, err := ownershipQuery(object)
	if err != nil {
		return OwnershipState{}, false, err
	}
	state := OwnershipState{Kind: object.Kind, Name: object.Name}
	err = db.QueryRowContext(ctx, query, args...).Scan(&state.Owner, &state.ExtensionOwned)
	if err == sql.ErrNoRows {
		return OwnershipState{}, false, nil
	}
	if err != nil {
		return OwnershipState{}, false, fmt.Errorf("inspect ownership %s: %w", ownershipKey(object.Kind, object.Name).Path(), err)
	}
	return state, true, nil
}

func inspectRoleCreatePrivilege(ctx context.Context, db *sql.DB, object Ownership, role string) (bool, error) {
	var roleExists bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = $1)`, role,
	).Scan(&roleExists); err != nil {
		return false, fmt.Errorf("inspect ownership authority role %q: %w", role, err)
	}
	if !roleExists {
		return false, nil
	}
	query := ""
	args := []any{role}
	switch object.Kind {
	case "database":
		query = `SELECT rolsuper OR rolcreatedb FROM pg_catalog.pg_roles WHERE rolname = $1`
	case "schema":
		query = `SELECT pg_catalog.has_database_privilege($1, current_database(), 'CREATE')`
	case "function", "procedure":
		open := strings.IndexByte(object.Name, '(')
		if open < 0 {
			return false, fmt.Errorf("routine name %q must be a safe schema-qualified identity signature", object.Name)
		}
		schema, _, err := splitQualifiedName(object.Name[:open])
		if err != nil {
			return false, err
		}
		query = `SELECT pg_catalog.has_schema_privilege($1, $2, 'CREATE')`
		args = append(args, schema)
	default:
		schema, _, err := splitQualifiedName(object.Name)
		if err != nil {
			return false, err
		}
		query = `SELECT pg_catalog.has_schema_privilege($1, $2, 'CREATE')`
		args = append(args, schema)
	}
	var allowed bool
	if err := db.QueryRowContext(ctx, query, args...).Scan(&allowed); err != nil {
		return false, fmt.Errorf("inspect role %q CREATE authority for %s: %w", role, ownershipKey(object.Kind, object.Name).Path(), err)
	}
	return allowed, nil
}

func inspectRoleDatabaseAuthority(ctx context.Context, db *sql.DB, role string) (bool, error) {
	var allowed bool
	if err := db.QueryRowContext(ctx,
		`SELECT rolsuper OR rolcreatedb FROM pg_catalog.pg_roles WHERE rolname = $1`, role,
	).Scan(&allowed); err != nil {
		return false, fmt.Errorf("inspect database authority for role %q: %w", role, err)
	}
	return allowed, nil
}

func ownershipQuery(object Ownership) (string, []any, error) {
	switch object.Kind {
	case "database":
		return `SELECT pg_get_userbyid(datdba), false FROM pg_catalog.pg_database WHERE datname = $1`, []any{object.Name}, nil
	case "schema":
		return `SELECT pg_get_userbyid(n.nspowner), EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid = 'pg_namespace'::regclass AND d.objid = n.oid AND d.deptype = 'e') FROM pg_catalog.pg_namespace n WHERE n.nspname = $1`, []any{object.Name}, nil
	case "table", "view", "materialized_view", "sequence":
		schema, name, err := splitQualifiedName(object.Name)
		if err != nil {
			return "", nil, err
		}
		kinds := map[string]string{"table": "ARRAY['r','p']", "view": "ARRAY['v']", "materialized_view": "ARRAY['m']", "sequence": "ARRAY['S']"}
		return fmt.Sprintf(`SELECT pg_get_userbyid(c.relowner), EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid AND d.deptype = 'e') FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $1 AND c.relname = $2 AND c.relkind = ANY(%s)`, kinds[object.Kind]), []any{schema, name}, nil
	case "type":
		schema, name, err := splitQualifiedName(object.Name)
		if err != nil {
			return "", nil, err
		}
		return `SELECT pg_get_userbyid(t.typowner), EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid = 'pg_type'::regclass AND d.objid = t.oid AND d.deptype = 'e') FROM pg_catalog.pg_type t JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = $1 AND t.typname = $2`, []any{schema, name}, nil
	case "function", "procedure":
		kind := "f"
		if object.Kind == "procedure" {
			kind = "p"
		}
		return `SELECT pg_get_userbyid(p.proowner), EXISTS (SELECT 1 FROM pg_catalog.pg_depend d WHERE d.classid = 'pg_proc'::regclass AND d.objid = p.oid AND d.deptype = 'e') FROM pg_catalog.pg_proc p WHERE p.oid = pg_catalog.to_regprocedure($1) AND p.prokind = $2`, []any{object.Name, kind}, nil
	default:
		return "", nil, fmt.Errorf("unsupported ownership kind %q", object.Kind)
	}
}

func splitQualifiedName(name string) (string, string, error) {
	schema, object, ok := strings.Cut(name, ".")
	if !ok || schema == "" || object == "" || strings.Contains(object, ".") {
		return "", "", fmt.Errorf("object name %q must be schema-qualified", name)
	}
	return schema, object, nil
}

func inspectDefaultPrivileges(ctx context.Context, db *sql.DB, refs []DefaultPrivilegeRef, destination map[DefaultPrivilegeRef]DefaultPrivilegeState) error {
	for _, ref := range refs {
		code := map[string]string{"tables": "r", "sequences": "S", "functions": "f", "types": "T", "schemas": "n"}[ref.ObjectType]
		rows, err := db.QueryContext(ctx, `
SELECT x.privilege_type, x.is_grantable
FROM pg_catalog.pg_roles owner_role
LEFT JOIN pg_catalog.pg_default_acl d
  ON d.defaclrole = owner_role.oid AND d.defaclnamespace = 0 AND d.defaclobjtype = $2::"char"
CROSS JOIN LATERAL aclexplode(COALESCE(d.defaclacl, acldefault($2::"char", owner_role.oid))) x
WHERE owner_role.rolname = $1
  AND CASE WHEN x.grantee = 0 THEN 'PUBLIC' ELSE pg_get_userbyid(x.grantee) END = $3
ORDER BY x.privilege_type`, ref.Owner, code, ref.Grantee)
		if err != nil {
			return fmt.Errorf("inspect global default privilege %s: %w", defaultPrivilegeKey(ref.Owner, ref.ObjectType, ref.Grantee).Path(), err)
		}
		state := DefaultPrivilegeState{Owner: ref.Owner, ObjectType: ref.ObjectType, Grantee: ref.Grantee, Privileges: make(map[string]bool)}
		for rows.Next() {
			var privilege string
			var grantable bool
			if err := rows.Scan(&privilege, &grantable); err != nil {
				rows.Close()
				return fmt.Errorf("scan global default privilege: %w", err)
			}
			state.Privileges[privilege] = grantable
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("inspect global default privilege: %w", err)
		}
		if len(state.Privileges) > 0 {
			destination[defaultPrivilegeKey(ref.Owner, ref.ObjectType, ref.Grantee)] = state
		}
	}
	return nil
}
