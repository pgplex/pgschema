package apply

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/globalstate"
	"github.com/pgplex/pgschema/internal/plan"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

func TestGlobalStatePlanIsReadOnlyAndSavedPlanConverges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	conn, host, port, database, user, password := testutil.ConnectToPostgres(t, target)
	defer conn.Close()

	_, err := conn.ExecContext(ctx, `
CREATE ROLE app_login NOLOGIN CONNECTION LIMIT -1;
CREATE ROLE provider_admin LOGIN;
CREATE ROLE unmanaged_role LOGIN;
CREATE ROLE retired_role;
`)
	require.NoError(t, err)

	majorVersion, err := detectPostgresMajorVersion(conn)
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, []byte(`
CREATE TABLE documents (id bigint PRIMARY KEY);
GRANT SELECT ON documents TO app_login;
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
CREATE POLICY documents_group ON documents FOR SELECT TO app_group USING (true);
`), 0o600))
	globalFile := filepath.Join(dir, "global.toml")
	membershipOptions := ""
	if majorVersion >= 16 {
		membershipOptions = "inherit = false\nset = true\n"
	}
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1

[[roles]]
name = "app_group"

[[roles]]
name = "app_login"
login = true
connection_limit = 12
valid_until = "2030-01-02T03:04:05Z"
configuration = { statement_timeout = "5s" }

[[roles]]
name = "provider_admin"
state = "external"

[[roles]]
name = "retired_role"
state = "absent"

[[memberships]]
role = "app_group"
member = "app_login"
`+membershipOptions+`
[[ownership]]
kind = "database"
name = "testdb"
owner = "app_group"

[[ownership]]
kind = "schema"
name = "public"
owner = "app_group"

[[ownership]]
kind = "table"
name = "public.documents"
owner = "app_group"

[[default_privileges]]
owner = "app_group"
object_type = "tables"
grantee = "app_login"
privileges = ["SELECT"]

[[default_privileges]]
owner = "provider_admin"
object_type = "functions"
grantee = "PUBLIC"
state = "absent"

[[default_privileges]]
owner = "app_login"
object_type = "types"
grantee = "PUBLIC"
privileges = ["USAGE"]
`), 0o600))

	manifest, err := globalstate.LoadManifest(globalFile)
	require.NoError(t, err)
	selection := globalstate.SelectionFor(manifest)
	before, err := globalstate.Inspect(ctx, conn, selection, majorVersion)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"USAGE": false},
		before.DefaultPrivileges[globalstate.DefaultPrivilegeRef{Owner: "app_login", ObjectType: "types", Grantee: "PUBLIC"}].Privileges,
		"a present role without a pg_default_acl row must expose PostgreSQL's implicit PUBLIC baseline")

	config := &planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: user, Password: password,
		Schema: "public", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-global-state-test", SSLMode: "disable",
	}
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	afterPlan, err := globalstate.Inspect(ctx, conn, selection, majorVersion)
	require.NoError(t, err)
	require.Equal(t, before, afterPlan, "planning must not mutate target cluster-global state")
	require.Equal(t, []string{
		"CREATE ROLE app_group WITH NOSUPERUSER NOLOGIN INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT -1 VALID UNTIL E'infinity'",
		"ALTER ROLE app_login WITH NOSUPERUSER LOGIN INHERIT NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 12 VALID UNTIL E'2030-01-02T03:04:05Z'; ALTER ROLE app_login RESET ALL; ALTER ROLE app_login SET statement_timeout TO E'5s'",
	}, []string{migrationPlan.Groups[0].Steps[0].SQL, migrationPlan.Groups[0].Steps[1].SQL})
	require.NotNil(t, migrationPlan.SourceGlobalFingerprint)
	require.Contains(t, migrationPlan.HumanColored(false), "Roles:")
	require.Contains(t, migrationPlan.HumanColored(false), "Role memberships:")

	encoded, err := migrationPlan.ToJSON()
	require.NoError(t, err)
	savedPlan, err := plan.FromJSON([]byte(encoded))
	require.NoError(t, err)
	_, err = conn.ExecContext(ctx, "ALTER ROLE provider_admin NOLOGIN")
	require.NoError(t, err)
	err = ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: user, Password: password,
		Schema: "public", Plan: savedPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-global-state-test", SSLMode: "disable",
	}, nil)
	require.ErrorContains(t, err, "global state fingerprint mismatch")
	var appGroupExists bool
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'app_group')").Scan(&appGroupExists))
	require.False(t, appGroupExists, "fingerprint mismatch must fail before the first global-state mutation")
	_, err = conn.ExecContext(ctx, "ALTER ROLE provider_admin LOGIN")
	require.NoError(t, err)

	require.NoError(t, ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: user, Password: password,
		Schema: "public", Plan: savedPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-global-state-test", SSLMode: "disable",
	}, nil))

	var login bool
	var connectionLimit int
	require.NoError(t, conn.QueryRowContext(ctx, `
SELECT rolcanlogin, rolconnlimit FROM pg_catalog.pg_roles WHERE rolname = 'app_login'
`).Scan(&login, &connectionLimit))
	require.True(t, login)
	require.Equal(t, 12, connectionLimit)
	var membershipCount int
	require.NoError(t, conn.QueryRowContext(ctx, `
SELECT count(*)
FROM pg_catalog.pg_auth_members m
JOIN pg_catalog.pg_roles granted ON granted.oid = m.roleid
JOIN pg_catalog.pg_roles member ON member.oid = m.member
WHERE granted.rolname = 'app_group' AND member.rolname = 'app_login'
`).Scan(&membershipCount))
	require.Equal(t, 1, membershipCount)
	var databaseOwner, schemaOwner, tableOwner string
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_get_userbyid(datdba) FROM pg_database WHERE datname = current_database()").Scan(&databaseOwner))
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_get_userbyid(nspowner) FROM pg_namespace WHERE nspname = 'public'").Scan(&schemaOwner))
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'public.documents'::regclass").Scan(&tableOwner))
	require.Equal(t, "app_group", databaseOwner)
	require.Equal(t, "app_group", schemaOwner)
	require.Equal(t, "app_group", tableOwner)
	var defaultSelect bool
	require.NoError(t, conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) x
  WHERE d.defaclnamespace = 0 AND pg_get_userbyid(d.defaclrole) = 'app_group'
    AND pg_get_userbyid(x.grantee) = 'app_login' AND x.privilege_type = 'SELECT'
)`).Scan(&defaultSelect))
	require.True(t, defaultSelect)
	var implicitExecute bool
	require.NoError(t, conn.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM pg_roles owner_role
  LEFT JOIN pg_default_acl d ON d.defaclrole = owner_role.oid
    AND d.defaclnamespace = 0 AND d.defaclobjtype = 'f'
  CROSS JOIN LATERAL aclexplode(COALESCE(d.defaclacl, acldefault('f', owner_role.oid))) x
  WHERE owner_role.rolname = 'provider_admin' AND x.grantee = 0
    AND x.privilege_type = 'EXECUTE'
)`).Scan(&implicitExecute))
	require.False(t, implicitExecute, "explicit absent state must override PostgreSQL's implicit PUBLIC EXECUTE baseline")
	var unmanagedLogin bool
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT rolcanlogin FROM pg_catalog.pg_roles WHERE rolname = 'unmanaged_role'").Scan(&unmanagedLogin))
	require.True(t, unmanagedLogin)
	var retiredExists bool
	require.NoError(t, conn.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = 'retired_role')").Scan(&retiredExists))
	require.False(t, retiredExists)

	replan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	require.False(t, replan.HasAnyChanges(), "replan after applying saved plan must have zero steps:\n%s", replan.HumanColored(false))
}

func TestGlobalStateNonSuperuserAuthorityConverges(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()

	_, err := admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN CREATEDB CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE app_login LOGIN;
ALTER DATABASE testdb OWNER TO deployer;
ALTER SCHEMA public OWNER TO deployer;
`)
	require.NoError(t, err)
	deployer, err := util.Connect(&util.ConnectionConfig{
		Host: host, Port: port, Database: database, User: "deployer", Password: "deployer-pass",
		SSLMode: "disable", ApplicationName: "pgschema-global-state-authority-test",
	})
	require.NoError(t, err)
	defer deployer.Close()

	majorVersion, err := detectPostgresMajorVersion(deployer)
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, []byte("CREATE TABLE documents (id bigint PRIMARY KEY);\n"), 0o600))
	globalFile := filepath.Join(dir, "global.toml")
	membershipOptions := "admin = true\n"
	if majorVersion >= 16 {
		membershipOptions += "inherit = false\nset = true\n"
	}
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1

[[roles]]
name = "app_owner"
createdb = true

[[roles]]
name = "app_login"
state = "external"

[[roles]]
name = "deployer"
state = "external"

[[memberships]]
role = "app_owner"
member = "deployer"
`+membershipOptions+`
[[ownership]]
kind = "database"
name = "testdb"
owner = "app_owner"

[[ownership]]
kind = "schema"
name = "public"
owner = "app_owner"

[[ownership]]
kind = "table"
name = "public.documents"
owner = "app_owner"

[[default_privileges]]
owner = "app_owner"
object_type = "functions"
grantee = "PUBLIC"
state = "absent"
`), 0o600))

	manifest, err := globalstate.LoadManifest(globalFile)
	require.NoError(t, err)
	selection := globalstate.SelectionFor(manifest)
	before, err := globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	config := &planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "public", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-global-state-authority-test", SSLMode: "disable",
	}
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	afterPlan, err := globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	require.Equal(t, before, afterPlan, "authority planning must not mutate target global state")

	applyErr := ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "public", Plan: migrationPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-global-state-authority-test", SSLMode: "disable",
	}, nil)
	require.NoError(t, applyErr, migrationPlan.ToSQL(plan.SQLFormatRaw))
	var owner string
	require.NoError(t, deployer.QueryRowContext(ctx,
		"SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'public.documents'::regclass").Scan(&owner))
	require.Equal(t, "app_owner", owner)
	var canSet bool
	setPrivilege := "SET"
	if majorVersion < 16 {
		setPrivilege = "MEMBER"
	}
	require.NoError(t, deployer.QueryRowContext(ctx,
		"SELECT pg_has_role(current_user, 'app_owner', $1)", setPrivilege).Scan(&canSet))
	require.True(t, canSet)

	replan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	require.False(t, replan.HasAnyChanges(), "non-superuser replan must have zero steps:\n%s", replan.HumanColored(false))
}
