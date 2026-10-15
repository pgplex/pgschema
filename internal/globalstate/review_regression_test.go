package globalstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReviewSelfAuthorityRevocationRunsAfterDependentOwnership(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_owner", State: StateExternal},
			{
				Name: "deployer", State: StatePresent, Login: true, Inherit: true,
				ConnectionLimit: -1,
			},
		},
		Memberships: []Membership{{Role: "app_owner", Member: "deployer", State: StateAbsent, Inherit: true, Set: true}},
		Ownership:   []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles: map[string]bool{
			"app_owner": true,
			"deployer":  true,
		},
		SessionSetRoles: map[string]bool{"app_owner": true},
		Roles: map[string]RoleState{
			"app_owner": {Name: "app_owner"},
			"deployer": {
				Name: "deployer", Login: true, Inherit: true, CreateRole: true,
				ConnectionLimit: -1, ValidUntil: "infinity",
			},
		},
		Memberships: map[MembershipRef]MembershipState{
			MembershipKey("app_owner", "deployer"): {Role: "app_owner", Member: "deployer", Admin: false, Inherit: true, Set: true},
		},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("table", "public.documents"): {Kind: "table", Name: "public.documents", Owner: "deployer"},
		},
		NewOwnerCreatePrivileges: map[OwnershipRef]bool{
			ownershipKey("table", "public.documents"): true,
		},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 3)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role_membership", changes[1].Type)
	require.Equal(t, "role", changes[2].Type)
}

func TestReviewRoleLiteralsAreIndependentOfStandardConformingStrings(t *testing.T) {
	manifest := Manifest{Version: 1, Roles: []Role{{
		Name: "app", State: StatePresent, Inherit: true, ConnectionLimit: -1,
		Configuration: map[string]string{"application_name": `path\with'quote`},
	}}}
	current := Snapshot{SessionRole: RoleState{Name: "postgres", Superuser: true}, Roles: map[string]RoleState{}}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Contains(t, changes[0].SQL, `E'path\\with''quote'`)
}

func TestReviewManifestRejectsDuplicatePrivileges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "global.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
version = 1

[[roles]]
name = "owner"
state = "external"

[[roles]]
name = "reader"
state = "external"

[[default_privileges]]
owner = "owner"
object_type = "tables"
grantee = "reader"
privileges = ["select", "SELECT"]
`), 0o600))

	_, err := LoadManifest(path)
	require.ErrorContains(t, err, "duplicate privilege")
}

func TestReviewManifestRejectsReferencesToAbsentRoles(t *testing.T) {
	tests := map[string]string{
		"ownership": `
version = 1

[[roles]]
name = "retired"
state = "absent"

[[ownership]]
kind = "schema"
name = "public"
owner = "retired"
`,
		"membership": `
version = 1

[[roles]]
name = "retired"
state = "absent"

[[roles]]
name = "active"
state = "external"

[[memberships]]
role = "retired"
member = "active"
`,
		"default privilege": `
version = 1

[[roles]]
name = "retired"
state = "absent"

[[default_privileges]]
owner = "retired"
object_type = "functions"
grantee = "PUBLIC"
state = "absent"
`,
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "global.toml")
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
			_, err := LoadManifest(path)
			require.ErrorContains(t, err, `role "retired" is declared absent`)
		})
	}
}

func TestReviewAuthorityLookupSupportsSlashInRoleNames(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "owner/with/slash", State: StateExternal},
			{Name: "member", State: StateExternal},
		},
		Memberships: []Membership{{
			Role: "owner/with/slash", Member: "member", State: StatePresent,
			Admin: true, Inherit: true, Set: true,
		}},
	}
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles: map[string]bool{"owner/with/slash": true},
		Roles: map[string]RoleState{
			"owner/with/slash": {Name: "owner/with/slash"},
			"member":           {Name: "member"},
		},
		Memberships: map[MembershipRef]MembershipState{},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Contains(t, changes[0].SQL, `"owner/with/slash"`)
}

func TestReviewCompositeKeysDoNotCollideOnSlash(t *testing.T) {
	require.NotEqual(t, MembershipKey("a/b", "c"), MembershipKey("a", "b/c"))
	require.NotEqual(t, ownershipKey("table/a", "b"), ownershipKey("table", "a/b"))
	require.NotEqual(t,
		defaultPrivilegeKey("a/b", "tables", "c"),
		defaultPrivilegeKey("a", "b/tables", "c"),
	)
}

func TestReviewUnchangedSchemaOwnerRequiresExecutionRoleAuthority(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "deployer", State: StateExternal},
			{Name: "app_owner", State: StateExternal},
		},
		Ownership: []Ownership{{Kind: "schema", Name: "managed", Owner: "app_owner"}},
	}
	current := Snapshot{
		SessionRole:     RoleState{Name: "deployer"},
		SessionSetRoles: map[string]bool{},
		Roles: map[string]RoleState{
			"deployer":  {Name: "deployer"},
			"app_owner": {Name: "app_owner"},
		},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("schema", "managed"): {Kind: "schema", Name: "managed", Owner: "app_owner"},
		},
	}

	err := ValidateExecutionRole(manifest, current, "app_owner", 18)
	require.ErrorContains(t, err, `session role "deployer" lacks SET authority on schema execution role "app_owner"`)

	manifest.Memberships = []Membership{{
		Role: "app_owner", Member: "deployer", State: StatePresent,
		Admin: false, Inherit: false, Set: true,
	}}
	require.NoError(t, ValidateExecutionRole(manifest, current, "app_owner", 18))
}

func TestReviewSuperuserLossRunsAfterDependentChanges(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "deployer", State: StatePresent, Login: true, Inherit: true, ConnectionLimit: -1},
			{Name: "app_owner", State: StateExternal},
		},
		Ownership: []Ownership{{Kind: "table", Name: "public.documents", Owner: "app_owner"}},
	}
	current := Snapshot{
		SessionRole: RoleState{Name: "deployer", Superuser: true},
		Roles: map[string]RoleState{
			"deployer":  {Name: "deployer", Superuser: true, Login: true, Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
			"app_owner": {Name: "app_owner"},
		},
		Ownership: map[OwnershipRef]OwnershipState{
			ownershipKey("table", "public.documents"): {Kind: "table", Name: "public.documents", Owner: "deployer"},
		},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role", changes[1].Type)
	require.Equal(t, "final", changes[1].Phase)
}

func TestReviewNonSessionAuthorityReductionRunsAfterOwnershipTransfer(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "deployer", State: StateExternal},
			{Name: "old_owner", State: StatePresent, Inherit: true, ConnectionLimit: -1},
			{Name: "new_owner", State: StateExternal},
		},
		Ownership: []Ownership{{Kind: "database", Name: "app", Owner: "new_owner"}},
	}
	ref := ownershipKey("database", "app")
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer", CreateRole: true, CreateDB: true},
		SessionAdminRoles: map[string]bool{"old_owner": true},
		SessionSetRoles:   map[string]bool{"old_owner": true, "new_owner": true},
		OwnerSetRoles:     map[RoleTransition]bool{{From: "old_owner", To: "new_owner"}: true},
		Roles: map[string]RoleState{
			"deployer":  {Name: "deployer", CreateRole: true, CreateDB: true},
			"old_owner": {Name: "old_owner", CreateDB: true, Inherit: true, ConnectionLimit: -1, ValidUntil: "infinity"},
			"new_owner": {Name: "new_owner", CreateDB: true},
		},
		Ownership:                     map[OwnershipRef]OwnershipState{ref: {Kind: "database", Name: "app", Owner: "old_owner"}},
		NewOwnerCreatePrivileges:      map[OwnershipRef]bool{ref: true},
		CurrentOwnerDatabaseAuthority: map[OwnershipRef]bool{ref: true},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role", changes[1].Type)
	require.Equal(t, "final", changes[1].Phase)
}

func TestReviewMembershipWithAdminOptionDoesNotRequireCreateRole(t *testing.T) {
	manifest := Manifest{
		Version:     1,
		Roles:       []Role{{Name: "app_group", State: StateExternal}, {Name: "app_login", State: StateExternal}},
		Memberships: []Membership{{Role: "app_group", Member: "app_login", State: StatePresent, Inherit: true, Set: true}},
	}
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer"},
		SessionAdminRoles: map[string]bool{"app_group": true},
		Roles:             map[string]RoleState{"app_group": {Name: "app_group"}, "app_login": {Name: "app_login"}},
		Memberships:       map[MembershipRef]MembershipState{},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, "role_membership", changes[0].Type)
}

func TestReviewMembershipAuthorityReductionRunsAfterOwnershipTransfer(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "deployer", State: StateExternal},
			{Name: "old_owner", State: StateExternal},
			{Name: "new_owner", State: StateExternal},
		},
		Memberships: []Membership{{Role: "new_owner", Member: "old_owner", State: StateAbsent, Inherit: true, Set: true}},
		Ownership:   []Ownership{{Kind: "table", Name: "public.documents", Owner: "new_owner"}},
	}
	ref := ownershipKey("table", "public.documents")
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer"},
		SessionAdminRoles: map[string]bool{"new_owner": true},
		SessionSetRoles:   map[string]bool{"old_owner": true, "new_owner": true},
		OwnerSetRoles:     map[RoleTransition]bool{{From: "old_owner", To: "new_owner"}: true},
		Roles: map[string]RoleState{
			"deployer":  {Name: "deployer"},
			"old_owner": {Name: "old_owner"},
			"new_owner": {Name: "new_owner"},
		},
		Memberships: map[MembershipRef]MembershipState{
			MembershipKey("new_owner", "old_owner"): {
				Role: "new_owner", Member: "old_owner", Inherit: true, Set: true,
				Grantors: []string{"deployer"},
			},
		},
		Ownership:                map[OwnershipRef]OwnershipState{ref: {Kind: "table", Name: "public.documents", Owner: "old_owner"}},
		NewOwnerCreatePrivileges: map[OwnershipRef]bool{ref: true},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role_membership", changes[1].Type)
	require.Equal(t, "final", changes[1].Phase)

	manifest.Memberships[0].State = StatePresent
	manifest.Memberships[0].Set = false
	changes, err = PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role_membership", changes[1].Type)
	require.Equal(t, "final", changes[1].Phase)
}

func TestReviewSessionAdminRevocationRunsAfterOtherMembershipRevokes(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_group", State: StateExternal},
			{Name: "deployer", State: StateExternal},
			{Name: "z_user", State: StateExternal},
		},
		Memberships: []Membership{
			{Role: "app_group", Member: "deployer", State: StateAbsent, Inherit: true, Set: true},
			{Role: "app_group", Member: "z_user", State: StateAbsent, Inherit: true, Set: true},
		},
	}
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer"},
		SessionAdminRoles: map[string]bool{"app_group": true},
		Roles: map[string]RoleState{
			"app_group": {Name: "app_group"}, "deployer": {Name: "deployer"}, "z_user": {Name: "z_user"},
		},
		Memberships: map[MembershipRef]MembershipState{
			MembershipKey("app_group", "deployer"): {
				Role: "app_group", Member: "deployer", Admin: true, Inherit: true, Set: true,
			},
			MembershipKey("app_group", "z_user"): {
				Role: "app_group", Member: "z_user", Inherit: true, Set: true,
			},
		},
	}

	changes, err := PlanChanges(manifest, current, 15)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "z_user", changes[0].Membership.Member)
	require.Equal(t, "deployer", changes[1].Membership.Member)
}

func TestReviewRefusesDropRoleWithUnmanagedDependencies(t *testing.T) {
	manifest := Manifest{Version: 1, Roles: []Role{{Name: "retired", State: StateAbsent}}}
	current := Snapshot{
		SessionRole:          RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles:    map[string]bool{"retired": true},
		Roles:                map[string]RoleState{"retired": {Name: "retired"}},
		RoleDependencyCounts: map[string]int{"retired": 1},
	}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `role "retired" still owns objects or holds privileges outside the managed selection`)
}

func TestReviewAllowsDropRoleAfterManagedOwnershipRelease(t *testing.T) {
	manifest := Manifest{
		Version:   1,
		Roles:     []Role{{Name: "retired", State: StateAbsent}, {Name: "active", State: StateExternal}},
		Ownership: []Ownership{{Kind: "table", Name: "public.documents", Owner: "active"}},
	}
	ref := ownershipKey("table", "public.documents")
	current := Snapshot{
		SessionRole:          RoleState{Name: "postgres", Superuser: true},
		Roles:                map[string]RoleState{"retired": {Name: "retired"}, "active": {Name: "active"}},
		Ownership:            map[OwnershipRef]OwnershipState{ref: {Kind: "table", Name: "public.documents", Owner: "retired"}},
		RoleDependencyCounts: map[string]int{"retired": 1},
	}

	changes, err := PlanChanges(manifest, current, 18)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	require.Equal(t, "ownership", changes[0].Type)
	require.Equal(t, "role", changes[1].Type)
}

func TestReviewRejectsForeignGrantorAuthorityReduction(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "app_group", State: StateExternal},
			{Name: "app_login", State: StateExternal},
		},
		Memberships: []Membership{{
			Role: "app_group", Member: "app_login", State: StatePresent,
			Admin: false, Inherit: true, Set: true,
		}},
	}
	current := Snapshot{
		SessionRole:       RoleState{Name: "deployer", CreateRole: true},
		SessionAdminRoles: map[string]bool{"app_group": true},
		Roles: map[string]RoleState{
			"app_group": {Name: "app_group"},
			"app_login": {Name: "app_login"},
		},
		Memberships: map[MembershipRef]MembershipState{
			MembershipKey("app_group", "app_login"): {
				Role: "app_group", Member: "app_login", Admin: true, Inherit: true, Set: true,
				Grantors: []string{"provider_admin"},
			},
		},
	}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `membership "9:app_group9:app_login" is owned by foreign grantor "provider_admin" and cannot be safely altered`)

	manifest.Memberships[0].State = StateAbsent
	_, err = PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `membership "9:app_group9:app_login" is owned by foreign grantor "provider_admin" and cannot be safely revoked`)
}

func TestReviewOwnershipTransferValidatesEffectiveExecutorAndNewOwner(t *testing.T) {
	manifest := Manifest{
		Version: 1,
		Roles: []Role{
			{Name: "deployer", State: StateExternal},
			{Name: "old_owner", State: StateExternal},
			{Name: "new_owner", State: StateExternal},
		},
		Ownership: []Ownership{{Kind: "database", Name: "app", Owner: "new_owner"}},
	}
	ref := ownershipKey("database", "app")
	current := Snapshot{
		SessionRole:     RoleState{Name: "deployer", CreateDB: true},
		SessionSetRoles: map[string]bool{"old_owner": true, "new_owner": true},
		OwnerSetRoles:   map[RoleTransition]bool{{From: "old_owner", To: "new_owner"}: true},
		Roles: map[string]RoleState{
			"deployer":  {Name: "deployer", CreateDB: true},
			"old_owner": {Name: "old_owner"},
			"new_owner": {Name: "new_owner", CreateDB: true},
		},
		Ownership:                     map[OwnershipRef]OwnershipState{ref: {Kind: "database", Name: "app", Owner: "old_owner"}},
		NewOwnerCreatePrivileges:      map[OwnershipRef]bool{ref: true},
		CurrentOwnerDatabaseAuthority: map[OwnershipRef]bool{ref: false},
	}

	_, err := PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `current owner role "old_owner" lacks CREATEDB required to change database ownership`)

	current.CurrentOwnerDatabaseAuthority[ref] = true
	current.NewOwnerCreatePrivileges[ref] = false
	current.Roles["new_owner"] = RoleState{Name: "new_owner"}
	_, err = PlanChanges(manifest, current, 18)
	require.ErrorContains(t, err, `new owner role "new_owner" lacks CREATEDB required to change database ownership`)

	current.NewOwnerCreatePrivileges[ref] = true
	_, err = PlanChanges(manifest, current, 18)
	require.NoError(t, err)
}
