package plan

import (
	"context"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/internal/globalstate"
	internalplan "github.com/pgplex/pgschema/internal/plan"
	"github.com/stretchr/testify/require"
)

func TestReviewSuperuserKeepsSchemaExecutionAuthority(t *testing.T) {
	current := globalstate.Snapshot{
		SessionRole: globalstate.RoleState{Name: "postgres", Superuser: true},
		Ownership: map[globalstate.OwnershipRef]globalstate.OwnershipState{
			{Kind: "schema", Name: "managed"}: {Kind: "schema", Name: "managed", Owner: "app_owner"},
		},
	}
	require.Empty(t, schemaExecutionRole(current, "managed"))
}

func TestReviewSuperuserDoesNotAssumeAffectedObjectOwner(t *testing.T) {
	p := &internalplan.Plan{Groups: []internalplan.ExecutionGroup{{Steps: []internalplan.Step{{
		Type: "table.column", Operation: "alter", Path: "managed.documents.title",
	}}}}}
	current := globalstate.Snapshot{SessionRole: globalstate.RoleState{Name: "postgres", Superuser: true}}

	require.NoError(t, setSchemaExecutionRoles(context.Background(), nil, p, globalstate.Manifest{}, current, "", "managed", 18))
	require.Empty(t, p.Groups[0].ExecutionRole)
}

func TestReviewDefaultPrivilegeUsesDeclaredOwner(t *testing.T) {
	p := &internalplan.Plan{Groups: []internalplan.ExecutionGroup{{Steps: []internalplan.Step{{
		Type: "default_privilege", Operation: "create", Path: "default_privileges.deployer.TABLES.app_reader",
		OwnerIdentity: diff.OwnerIdentity{Kind: diff.OwnerIdentityDefaultPrivilege, Role: "deployer"},
	}}}}}
	current := globalstate.Snapshot{SessionRole: globalstate.RoleState{Name: "deployer"}}

	require.NoError(t, setSchemaExecutionRoles(context.Background(), nil, p, globalstate.Manifest{}, current, "app_owner", "managed", 18))
	require.Empty(t, p.Groups[0].ExecutionRole)
}

func TestReviewGenericCommentFailsBeforeMutation(t *testing.T) {
	p := &internalplan.Plan{Groups: []internalplan.ExecutionGroup{{Steps: []internalplan.Step{{
		Type: "comment", Operation: "alter", Path: "opaque",
	}}}}}
	current := globalstate.Snapshot{SessionRole: globalstate.RoleState{Name: "deployer"}}

	err := setSchemaExecutionRoles(context.Background(), nil, p, globalstate.Manifest{}, current, "app_owner", "managed", 18)
	require.ErrorContains(t, err, `cannot safely resolve execution owner for generic comment "opaque"`)
}

func TestReviewRetiringExecutionRoleCannotOwnCreatedObjects(t *testing.T) {
	manifest := globalstate.Manifest{Version: 1, Roles: []globalstate.Role{{Name: "retiring", State: globalstate.StateAbsent}}}
	p := &internalplan.Plan{Groups: []internalplan.ExecutionGroup{{Steps: []internalplan.Step{{
		Type: "table", Operation: "create", Path: "managed.documents",
	}}}}}

	err := validateRetiringExecutionRole(manifest, p, "retiring")
	require.ErrorContains(t, err, `retiring schema execution role "retiring" would own newly created objects`)
}
