package globalstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadManifestDefaultsAndRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "global.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
version = 1

[[roles]]
name = "app_login"
login = true

[[roles]]
name = "provider_admin"
state = "external"

[[memberships]]
role = "app_group"
member = "app_login"
`), 0o600))

	manifest, err := LoadManifest(path)
	require.NoError(t, err)
	require.Equal(t, Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_login", State: StatePresent, Login: true, Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
			{Name: "provider_admin", State: StateExternal, Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
		},
		Memberships: []Membership{{
			Role: "app_group", Member: "app_login", State: StatePresent, Inherit: true, Set: true,
		}},
	}, manifest)

	require.NoError(t, os.WriteFile(path, []byte("version = 1\nunknown = true\n"), 0o600))
	_, err = LoadManifest(path)
	require.ErrorContains(t, err, "unknown field")
}

func TestPlanChangesCreatesAndAltersRolesBeforeMemberships(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_group", State: StatePresent, Inherit: true, ConnectionLimit: -1},
			{Name: "app_login", State: StatePresent, Login: true, Inherit: true, ConnectionLimit: 12},
			{Name: "provider_admin", State: StateExternal, Inherit: true, ConnectionLimit: -1},
		},
		Memberships: []Membership{{
			Role: "app_group", Member: "app_login", State: StatePresent, Admin: false, Inherit: false, Set: true,
		}},
	}
	current := Snapshot{
		Roles: map[string]RoleState{
			"app_login":      {Name: "app_login", Inherit: true, ConnectionLimit: -1},
			"provider_admin": {Name: "provider_admin", Inherit: true, ConnectionLimit: -1},
		},
		Memberships: map[MembershipRef]MembershipState{},
	}

	changes, err := PlanChanges(manifest, current, 16)
	require.NoError(t, err)
	require.Equal(t, []Change{
		{
			SQL:       `CREATE ROLE app_group WITH NOSUPERUSER NOLOGIN INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT -1 VALID UNTIL E'infinity'`,
			Type:      "role",
			Operation: "create",
			Path:      "app_group",
			RoleName:  "app_group",
		},
		{
			SQL:       `ALTER ROLE app_login WITH NOSUPERUSER LOGIN INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 12 VALID UNTIL E'infinity'; ALTER ROLE app_login RESET ALL`,
			Type:      "role",
			Operation: "alter",
			Path:      "app_login",
			RoleName:  "app_login",
		},
		{
			SQL:        `GRANT app_group TO app_login WITH ADMIN FALSE, INHERIT FALSE, SET TRUE`,
			Type:       "role_membership",
			Operation:  "create",
			Path:       MembershipKey("app_group", "app_login").Path(),
			Membership: &MembershipRef{Role: "app_group", Member: "app_login"},
		},
	}, changes)
}

func TestPlanChangesPostgres15MembershipOptions(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_group", State: StateExternal, Inherit: true, ConnectionLimit: -1},
			{Name: "app_login", State: StateExternal, Inherit: true, ConnectionLimit: -1},
		},
		Memberships: []Membership{{
			Role: "app_group", Member: "app_login", State: StatePresent, Admin: false, Inherit: false, Set: true,
		}},
	}
	current := Snapshot{Roles: map[string]RoleState{
		"app_group": {Name: "app_group"},
		"app_login": {Name: "app_login"},
	}}

	_, err := PlanChanges(manifest, current, 15)
	require.ErrorContains(t, err, "PostgreSQL 16 or newer")

	manifest.Memberships[0].Inherit = true
	current.Memberships = map[MembershipRef]MembershipState{
		MembershipKey("app_group", "app_login"): {Role: "app_group", Member: "app_login", Admin: true, Inherit: true, Set: true},
	}
	changes, err := PlanChanges(manifest, current, 15)
	require.NoError(t, err)
	require.Equal(t, `REVOKE ADMIN OPTION FOR app_group FROM app_login`, changes[0].SQL)
}

func TestPlanChangesRejectsMaintainDefaultPrivilegeBeforePostgres17(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles:   []Role{{Name: "app_owner", State: StateExternal}},
		DefaultPrivileges: []DefaultPrivilege{{
			Owner: "app_owner", ObjectType: "tables", Grantee: "PUBLIC",
			Privileges: []string{"MAINTAIN"}, State: StatePresent,
		}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "postgres", Superuser: true},
		Roles:       map[string]RoleState{"app_owner": {Name: "app_owner"}},
	}

	_, err := PlanChanges(manifest, current, 16)
	require.ErrorContains(t, err, "PostgreSQL 17 or newer")
	changes, err := PlanChanges(manifest, current, 17)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Contains(t, changes[0].SQL, "GRANT MAINTAIN ON TABLES")
}

func TestPlanChangesValidatesExternalRolesAndLeavesUnmanagedRolesAlone(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles:   []Role{{Name: "provider_admin", State: StateExternal, Inherit: true, ConnectionLimit: -1}},
	}
	current := Snapshot{Roles: map[string]RoleState{
		"unmanaged": {Name: "unmanaged", Login: true},
	}}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `external role "provider_admin" does not exist`)

	current.Roles["provider_admin"] = RoleState{Name: "provider_admin", Login: true}
	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Empty(t, changes)
}

func TestFingerprintIsDeterministicAndScoped(t *testing.T) {
	selection := Selection{
		Roles:       []string{"app_login", "provider_admin"},
		Memberships: []MembershipRef{{Role: "app_group", Member: "app_login"}},
	}
	one := Snapshot{Roles: map[string]RoleState{
		"provider_admin": {Name: "provider_admin", Login: true},
		"app_login":      {Name: "app_login", Login: true},
	}}
	two := Snapshot{Roles: map[string]RoleState{
		"app_login":      {Name: "app_login", Login: true},
		"provider_admin": {Name: "provider_admin", Login: true},
		"unmanaged":      {Name: "unmanaged", Login: false},
	}}

	fingerprintOne, err := ComputeFingerprint(one, selection)
	require.NoError(t, err)
	fingerprintTwo, err := ComputeFingerprint(two, selection)
	require.NoError(t, err)
	require.Equal(t, fingerprintOne, fingerprintTwo)
}

func TestFingerprintIncludesOnlyRelevantApplyingAuthority(t *testing.T) {
	selection := Selection{
		Roles:     []string{"app_owner"},
		Ownership: []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
	}
	base := Snapshot{
		SessionRole:       RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles: map[string]bool{"app_owner": true},
		SessionSetRoles:   map[string]bool{"app_owner": true},
		Roles:             map[string]RoleState{"app_owner": {Name: "app_owner"}},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("table", "public.documents"): {Kind: "table", Name: "public.documents", Owner: "existing_owner"},
		},
	}
	withUnrelatedAuthority := base
	withUnrelatedAuthority.SessionAdminRoles = map[string]bool{"app_owner": true, "unmanaged": true}
	withUnrelatedAuthority.SessionSetRoles = map[string]bool{"app_owner": true, "unmanaged": true}

	baseFingerprint, err := ComputeFingerprint(base, selection)
	require.NoError(t, err)
	unrelatedFingerprint, err := ComputeFingerprint(withUnrelatedAuthority, selection)
	require.NoError(t, err)
	require.Equal(t, baseFingerprint.Hash, unrelatedFingerprint.Hash)

	withoutCurrentOwnerAuthority := base
	withoutCurrentOwnerAuthority.SessionSetRoles = map[string]bool{"app_owner": true}
	base.SessionSetRoles["existing_owner"] = true
	withCurrentOwnerAuthority, err := ComputeFingerprint(base, selection)
	require.NoError(t, err)
	withoutCurrentOwner, err := ComputeFingerprint(withoutCurrentOwnerAuthority, selection)
	require.NoError(t, err)
	require.NotEqual(t, withCurrentOwnerAuthority.Hash, withoutCurrentOwner.Hash)

	ref := ownershipKey("schema", "managed")
	selection.Ownership = []Ownership{{Kind: "schema", Name: "managed", Owner: "app_owner"}}
	withExecutorCreate := base
	withExecutorCreate.OwnershipExecutorCreatePrivileges = map[OwnershipRef]bool{ref: true}
	withoutExecutorCreate := base
	withoutExecutorCreate.OwnershipExecutorCreatePrivileges = map[OwnershipRef]bool{ref: false}
	withExecutorCreateFingerprint, err := ComputeFingerprint(withExecutorCreate, selection)
	require.NoError(t, err)
	withoutExecutorCreateFingerprint, err := ComputeFingerprint(withoutExecutorCreate, selection)
	require.NoError(t, err)
	require.NotEqual(t, withExecutorCreateFingerprint.Hash, withoutExecutorCreateFingerprint.Hash)
}

func TestPlanChangesConvergesRoleLifecycleOwnershipAndGlobalDefaults(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_owner", State: StatePresent, Inherit: true, ConnectionLimit: -1, ValidUntil: "2030-01-02T03:04:05Z", Configuration: map[string]string{"statement_timeout": "5s"}},
			{Name: "app_reader", State: StateExternal},
			{Name: "retired_role", State: StateAbsent},
		},
		Ownership:         []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
		DefaultPrivileges: []DefaultPrivilege{{Owner: "app_owner", ObjectType: "tables", Grantee: "app_reader", Privileges: []string{"SELECT"}, State: StatePresent}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "postgres", Superuser: true},
		Roles: map[string]RoleState{
			"app_owner":    {Name: "app_owner", Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
			"app_reader":   {Name: "app_reader"},
			"retired_role": {Name: "retired_role"},
		},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("table", "public.documents"): {Kind: "table", Name: "public.documents", Owner: "postgres"},
		},
		DefaultPrivileges: map[DefaultPrivilegeRef]DefaultPrivilegeState{},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Equal(t, `ALTER ROLE app_owner WITH NOSUPERUSER NOLOGIN INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT -1 VALID UNTIL E'2030-01-02T03:04:05Z'; ALTER ROLE app_owner RESET ALL; ALTER ROLE app_owner SET statement_timeout TO E'5s'`, changes[0].SQL)
	require.Equal(t, `ALTER TABLE public.documents OWNER TO app_owner`, changes[1].SQL)
	require.Equal(t, `ALTER DEFAULT PRIVILEGES FOR ROLE app_owner REVOKE ALL ON TABLES FROM app_reader; ALTER DEFAULT PRIVILEGES FOR ROLE app_owner GRANT SELECT ON TABLES TO app_reader`, changes[2].SQL)
	require.Equal(t, "final", changes[3].Phase)
	require.Equal(t, `DROP ROLE retired_role`, changes[3].SQL)
}

func TestPlanChangesRejectsRoleMutationWithoutAuthority(t *testing.T) {
	manifest := Manifest{Version: 1, Roles: []Role{{Name: "app", State: StatePresent, Inherit: true, ConnectionLimit: -1}}}
	current := Snapshot{SessionRole: RoleState{Name: "deployer"}, Roles: map[string]RoleState{}}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `session role "deployer" lacks CREATEROLE`)
}

func TestPlanChangesRevokesImplicitPublicFunctionExecute(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles:   []Role{{Name: "app_owner", State: StateExternal}},
		DefaultPrivileges: []DefaultPrivilege{{
			Owner: "app_owner", ObjectType: "functions", Grantee: "PUBLIC", State: StateAbsent,
		}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "postgres", Superuser: true},
		Roles:       map[string]RoleState{"app_owner": {Name: "app_owner"}},
		DefaultPrivileges: map[DefaultPrivilegeRef]DefaultPrivilegeState{
			defaultPrivilegeKey("app_owner", "functions", "PUBLIC"): {
				Owner: "app_owner", ObjectType: "functions", Grantee: "PUBLIC",
				Privileges: map[string]bool{"EXECUTE": false},
			},
		},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 1, "implicit baseline must be compared, not treated as an empty ACL")
	require.Equal(t, "ALTER DEFAULT PRIVILEGES FOR ROLE app_owner REVOKE ALL ON FUNCTIONS FROM PUBLIC", changes[0].SQL)
}

func TestPlanChangesRejectsUnprovenOwnershipAuthorityAndExtensionMembers(t *testing.T) {
	manifest := Manifest{
		Version:   1,
		Roles:     []Role{{Name: "app_owner", State: StateExternal}},
		Ownership: []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "deployer", CreateRole: true},
		Roles:       map[string]RoleState{"app_owner": {Name: "app_owner"}},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("table", "public.documents"): {Kind: "table", Name: "public.documents", Owner: "existing_owner"},
		},
	}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, "cannot assume current owner role")

	current.SessionRole.Superuser = true
	current.Ownership[ownershipKey("table", "public.documents")] = OwnershipState{
		Kind: "table", Name: "public.documents", Owner: "existing_owner", ExtensionOwned: true,
	}
	_, err = PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, "extension member")
}

func TestPlanChangesRequiresDeclaredSetMembershipForNewOwner(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_owner", State: StatePresent, Inherit: true, ConnectionLimit: -1},
			{Name: "deployer", State: StateExternal},
		},
		Ownership: []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
		DefaultPrivileges: []DefaultPrivilege{{
			Owner: "app_owner", ObjectType: "functions", Grantee: "PUBLIC", State: StateAbsent,
		}},
	}
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles: map[string]bool{},
		SessionSetRoles:   map[string]bool{},
		Roles:             map[string]RoleState{"deployer": {Name: "deployer"}},
		Ownership:         map[OwnershipRef]OwnershipState{},
		NewOwnerCreatePrivileges: map[OwnershipRef]bool{
			ownershipKey("table", "public.documents"): true,
		},
		DefaultPrivileges: map[DefaultPrivilegeRef]DefaultPrivilegeState{
			defaultPrivilegeKey("app_owner", "functions", "PUBLIC"): {
				Owner: "app_owner", ObjectType: "functions", Grantee: "PUBLIC",
				Privileges: map[string]bool{"EXECUTE": false},
			},
		},
	}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, "declare a SET-enabled membership")

	manifest.Memberships = []Membership{{
		Role: "app_owner", Member: "deployer", State: StatePresent, Admin: true, Inherit: false, Set: true,
	}}
	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 4)
	require.Equal(t, "role_membership", changes[1].Type)
	require.Equal(t, "ownership", changes[2].Type)
	require.Equal(t, "global_default_privilege", changes[3].Type)
	require.Contains(t, changes[3].SQL, "SET ROLE app_owner")
}

func TestPlanChangesConvergesAndProtectsSuperuserState(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles:   []Role{{Name: "app", State: StatePresent, Inherit: true, ConnectionLimit: -1}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "postgres", Superuser: true},
		Roles: map[string]RoleState{
			"app": {Name: "app", Superuser: true, Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
		},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Contains(t, changes[0].SQL, "NOSUPERUSER")

	current.SessionRole = RoleState{Name: "deployer", CreateRole: true}
	current.SessionAdminRoles = map[string]bool{"app": true}
	_, err = PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, "must be superuser")
}
