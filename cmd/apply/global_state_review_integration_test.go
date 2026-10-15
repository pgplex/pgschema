package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	planCmd "github.com/pgplex/pgschema/cmd/plan"
	"github.com/pgplex/pgschema/cmd/util"
	"github.com/pgplex/pgschema/internal/globalstate"
	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
	"github.com/stretchr/testify/require"
)

func TestReviewPlanCanAddSetAuthorityForExistingSchemaOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()

	_, err := admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN CREATEROLE PASSWORD 'deployer-pass';
SET ROLE deployer;
CREATE ROLE app_owner;
RESET ROLE;
CREATE SCHEMA managed AUTHORIZATION app_owner;
CREATE TABLE managed.documents (id bigint PRIMARY KEY);
ALTER TABLE managed.documents OWNER TO app_owner;
REVOKE ALL ON SCHEMA managed FROM PUBLIC;
`)
	require.NoError(t, err)
	deployer, err := util.Connect(&util.ConnectionConfig{
		Host: host, Port: port, Database: database, User: "deployer", Password: "deployer-pass",
		SSLMode: "disable", ApplicationName: "pgschema-review-schema-owner-test",
	})
	require.NoError(t, err)
	defer deployer.Close()

	majorVersion, err := detectPostgresMajorVersion(deployer)
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, []byte("CREATE TABLE documents (id bigint PRIMARY KEY, title text);\n"), 0o600))
	options := "admin = true\n"
	if majorVersion >= 16 {
		options += "inherit = false\nset = true\n"
	}
	globalFile := filepath.Join(dir, "global.toml")
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1

[[roles]]
name = "app_owner"
state = "external"

[[roles]]
name = "deployer"
state = "external"

[[memberships]]
role = "app_owner"
member = "deployer"
`+options+`
[[ownership]]
kind = "schema"
name = "managed"
owner = "app_owner"
`), 0o600))

	config := &planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "managed", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-review-schema-owner-test", SSLMode: "disable",
	}
	unauthorizedGlobalFile := filepath.Join(dir, "global-unauthorized.toml")
	require.NoError(t, os.WriteFile(unauthorizedGlobalFile, []byte(`
version = 1

[[roles]]
name = "app_owner"
state = "external"

[[roles]]
name = "deployer"
state = "external"

[[ownership]]
kind = "schema"
name = "managed"
owner = "app_owner"
`), 0o600))
	unauthorizedConfig := *config
	unauthorizedConfig.GlobalFile = unauthorizedGlobalFile
	_, err = planCmd.GeneratePlan(&unauthorizedConfig, sharedEmbeddedPG)
	require.ErrorContains(t, err, `lacks SET authority on schema execution role "app_owner"`)

	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	var titleExists bool
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_catalog.pg_attribute a
  JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'managed' AND c.relname = 'documents' AND a.attname = 'title'
)`).Scan(&titleExists))
	require.False(t, titleExists, "planning must not mutate the target")
	require.NoError(t, ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "managed", Plan: migrationPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-review-schema-owner-test", SSLMode: "disable",
	}, nil))
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_catalog.pg_attribute a
  JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
  JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
  WHERE n.nspname = 'managed' AND c.relname = 'documents' AND a.attname = 'title'
)`).Scan(&titleExists))
	require.True(t, titleExists)
	replan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	require.False(t, replan.HasAnyChanges(), "replan must converge after applying the planned SET relationship and schema DDL:\n%s", replan.HumanColored(false))
}

func TestReviewMixedOwnerSchemaUsesAffectedObjectOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	_, err := admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN NOINHERIT CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE app_owner;
CREATE ROLE "app.reader";
CREATE ROLE "column.reader";
CREATE SCHEMA managed AUTHORIZATION app_owner;
CREATE SCHEMA "managed.foo";
CREATE TYPE "managed.foo".custom_type AS ENUM ('one');
GRANT USAGE, CREATE ON SCHEMA managed TO deployer;
GRANT USAGE ON SCHEMA "managed.foo" TO deployer;
CREATE TABLE managed.documents (id bigint PRIMARY KEY);
ALTER TABLE managed.documents OWNER TO deployer;
CREATE TABLE managed."audit.log" (id bigint PRIMARY KEY);
ALTER TABLE managed."audit.log" OWNER TO deployer;
CREATE FUNCTION managed."calculate.dot"(value integer) RETURNS integer LANGUAGE sql AS 'SELECT value';
ALTER FUNCTION managed."calculate.dot"(integer) OWNER TO deployer;
CREATE FUNCTION managed."calculate.dot"(value text) RETURNS text LANGUAGE sql AS 'SELECT value';
ALTER FUNCTION managed."calculate.dot"(text) OWNER TO app_owner;
CREATE FUNCTION managed."custom.arg"(value "managed.foo".custom_type) RETURNS integer LANGUAGE sql AS 'SELECT 1';
ALTER FUNCTION managed."custom.arg"("managed.foo".custom_type) OWNER TO deployer;
CREATE PROCEDURE managed."custom.proc"(value "managed.foo".custom_type) LANGUAGE sql AS 'SELECT 1';
ALTER PROCEDURE managed."custom.proc"("managed.foo".custom_type) OWNER TO deployer;
CREATE FUNCTION managed."custom.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
ALTER FUNCTION managed."custom.transition"(bigint, "managed.foo".custom_type) OWNER TO deployer;
CREATE AGGREGATE managed."custom.aggregate" ("managed.foo".custom_type) (
  SFUNC = managed."custom.transition", STYPE = bigint, INITCOND = '0'
);
ALTER AGGREGATE managed."custom.aggregate" ("managed.foo".custom_type) OWNER TO deployer;
CREATE FUNCTION managed."ordered.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
ALTER FUNCTION managed."ordered.transition"(bigint, "managed.foo".custom_type) OWNER TO deployer;
CREATE AGGREGATE managed."ordered.dot" (ORDER BY "managed.foo".custom_type) (
  SFUNC = managed."ordered.transition", STYPE = bigint, INITCOND = '0'
);
ALTER AGGREGATE managed."ordered.dot" (ORDER BY "managed.foo".custom_type) OWNER TO deployer;
CREATE FUNCTION managed."hypothetical.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
ALTER FUNCTION managed."hypothetical.transition"(bigint, "managed.foo".custom_type) OWNER TO deployer;
CREATE AGGREGATE managed."hypothetical.dot" ("managed.foo".custom_type ORDER BY "managed.foo".custom_type) (
  SFUNC = managed."hypothetical.transition", STYPE = bigint, INITCOND = '0', HYPOTHETICAL
);
ALTER AGGREGATE managed."hypothetical.dot" ("managed.foo".custom_type ORDER BY "managed.foo".custom_type) OWNER TO deployer;
CREATE AGGREGATE managed."row.count" (*) (SFUNC = int8inc, STYPE = bigint, INITCOND = '0');
ALTER AGGREGATE managed."row.count" (*) OWNER TO deployer;
`)
	require.NoError(t, err)
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)
	grant := "GRANT app_owner TO deployer WITH ADMIN OPTION"
	if majorVersion >= 16 {
		grant = "GRANT app_owner TO deployer WITH ADMIN TRUE, INHERIT FALSE, SET TRUE"
	}
	_, err = admin.ExecContext(ctx, grant)
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, []byte(`
CREATE SCHEMA "managed.foo";
CREATE TYPE "managed.foo".custom_type AS ENUM ('one');
CREATE TABLE documents (id bigint PRIMARY KEY, title text);
CREATE TABLE "audit.log" (id bigint PRIMARY KEY, title text);
CREATE FUNCTION "calculate.dot"(value integer) RETURNS integer LANGUAGE sql AS 'SELECT value + 1';
CREATE FUNCTION "calculate.dot"(value text) RETURNS text LANGUAGE sql AS 'SELECT value';
CREATE FUNCTION "custom.arg"(value "managed.foo".custom_type) RETURNS integer LANGUAGE sql AS 'SELECT 2';
CREATE PROCEDURE "custom.proc"(value "managed.foo".custom_type) LANGUAGE sql AS 'SELECT 1';
COMMENT ON PROCEDURE "custom.proc"("managed.foo".custom_type) IS 'managed procedure';
CREATE FUNCTION "custom.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
CREATE AGGREGATE "custom.aggregate" ("managed.foo".custom_type) (
  SFUNC = "custom.transition", STYPE = bigint, INITCOND = '0'
);
COMMENT ON AGGREGATE "custom.aggregate" ("managed.foo".custom_type) IS 'managed aggregate';
CREATE FUNCTION "ordered.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
CREATE AGGREGATE "ordered.dot" (ORDER BY "managed.foo".custom_type) (
  SFUNC = "ordered.transition", STYPE = bigint, INITCOND = '0'
);
COMMENT ON AGGREGATE "ordered.dot" (ORDER BY "managed.foo".custom_type) IS 'managed ordered aggregate';
CREATE FUNCTION "hypothetical.transition"(state bigint, value "managed.foo".custom_type) RETURNS bigint LANGUAGE sql AS 'SELECT state + 1';
CREATE AGGREGATE "hypothetical.dot" ("managed.foo".custom_type ORDER BY "managed.foo".custom_type) (
  SFUNC = "hypothetical.transition", STYPE = bigint, INITCOND = '0', HYPOTHETICAL
);
COMMENT ON AGGREGATE "hypothetical.dot" ("managed.foo".custom_type ORDER BY "managed.foo".custom_type) IS 'managed hypothetical aggregate';
CREATE AGGREGATE "row.count" (*) (SFUNC = int8inc, STYPE = bigint, INITCOND = '0');
COMMENT ON AGGREGATE "row.count" (*) IS 'managed count';
GRANT SELECT ON TABLE "audit.log" TO "app.reader";
GRANT UPDATE (title) ON TABLE "audit.log" TO "column.reader";
GRANT EXECUTE ON FUNCTION "custom.arg"("managed.foo".custom_type) TO "app.reader";
REVOKE EXECUTE ON FUNCTION "custom.arg"("managed.foo".custom_type) FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE deployer IN SCHEMA managed GRANT SELECT ON TABLES TO "app.reader";
`), 0o600))
	globalFile := filepath.Join(dir, "global.toml")
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1
[[roles]]
name = "app_owner"
state = "external"
[[roles]]
name = "deployer"
state = "external"
[[roles]]
name = "app.reader"
state = "external"
[[roles]]
name = "column.reader"
state = "external"
[[ownership]]
kind = "schema"
name = "managed"
owner = "app_owner"
`), 0o600))
	config := &planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "managed", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-review-mixed-owner-test", SSLMode: "disable",
	}
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	require.NoError(t, ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "managed", Plan: migrationPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-review-mixed-owner-test", SSLMode: "disable",
	}, nil))
	var owner string
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT pg_get_userbyid(relowner) FROM pg_class WHERE oid = 'managed.documents'::regclass`).Scan(&owner))
	require.Equal(t, "deployer", owner)
	var canSelect bool
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT has_table_privilege('app.reader', 'managed."audit.log"', 'SELECT')`).Scan(&canSelect))
	require.True(t, canSelect)
	var canUpdateTitle bool
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT has_column_privilege('column.reader', 'managed."audit.log"', 'title', 'UPDATE')`).Scan(&canUpdateTitle))
	require.True(t, canUpdateTitle)
	var hasDefaultSelect bool
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT EXISTS (
  SELECT 1 FROM pg_default_acl d CROSS JOIN LATERAL aclexplode(d.defaclacl) x
  WHERE d.defaclnamespace = 'managed'::regnamespace
    AND pg_get_userbyid(d.defaclrole) = 'deployer'
    AND pg_get_userbyid(x.grantee) = 'app.reader'
    AND x.privilege_type = 'SELECT'
)`).Scan(&hasDefaultSelect))
	require.True(t, hasDefaultSelect)
	var calculated int
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT managed."calculate.dot"(1)`).Scan(&calculated))
	require.Equal(t, 2, calculated)
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT managed."custom.arg"('one'::"managed.foo".custom_type)`).Scan(&calculated))
	require.Equal(t, 2, calculated)
	var canExecuteCustom bool
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT has_function_privilege('app.reader', 'managed."custom.arg"("managed.foo".custom_type)', 'EXECUTE')`).Scan(&canExecuteCustom))
	require.True(t, canExecuteCustom)
	var publicCanExecuteCustom bool
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT has_function_privilege('public', 'managed."custom.arg"("managed.foo".custom_type)', 'EXECUTE')`).Scan(&publicCanExecuteCustom))
	require.False(t, publicCanExecuteCustom)
	var procedureComment string
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT obj_description('managed."custom.proc"("managed.foo".custom_type)'::regprocedure, 'pg_proc')`).Scan(&procedureComment))
	require.Equal(t, "managed procedure", procedureComment)
	var customAggregateComment string
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT obj_description('managed."custom.aggregate"("managed.foo".custom_type)'::regprocedure, 'pg_proc')`).Scan(&customAggregateComment))
	require.Equal(t, "managed aggregate", customAggregateComment)
	var orderedAggregateComment string
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT obj_description(p.oid, 'pg_proc')
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'managed' AND p.proname = 'ordered.dot' AND p.prokind = 'a'
`).Scan(&orderedAggregateComment))
	require.Equal(t, "managed ordered aggregate", orderedAggregateComment)
	var hypotheticalAggregateComment string
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT obj_description(p.oid, 'pg_proc')
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = 'managed' AND p.proname = 'hypothetical.dot' AND p.prokind = 'a'
`).Scan(&hypotheticalAggregateComment))
	require.Equal(t, "managed hypothetical aggregate", hypotheticalAggregateComment)
	var aggregateComment string
	require.NoError(t, admin.QueryRowContext(ctx, `SELECT obj_description('managed."row.count"()'::regprocedure, 'pg_proc')`).Scan(&aggregateComment))
	require.Equal(t, "managed count", aggregateComment)
}

func TestReviewRetiringSchemaOwnerCannotCreateBeforeDrop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	_, err := admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN NOINHERIT CREATEDB CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE retiring CREATEDB;
CREATE ROLE active CREATEDB;
ALTER DATABASE testdb OWNER TO retiring;
CREATE SCHEMA managed AUTHORIZATION retiring;
`)
	require.NoError(t, err)
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)
	grants := "GRANT retiring TO deployer WITH ADMIN OPTION; GRANT active TO deployer WITH ADMIN OPTION; GRANT active TO retiring"
	if majorVersion >= 16 {
		grants = "GRANT retiring TO deployer WITH ADMIN TRUE, INHERIT FALSE, SET TRUE; " +
			"GRANT active TO deployer WITH ADMIN TRUE, INHERIT FALSE, SET TRUE; " +
			"GRANT active TO retiring WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
	}
	_, err = admin.ExecContext(ctx, grants)
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, []byte("CREATE TABLE documents (id bigint PRIMARY KEY);\n"), 0o600))
	globalFile := filepath.Join(dir, "global.toml")
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1
[[roles]]
name = "retiring"
state = "absent"
[[roles]]
name = "active"
state = "external"
[[roles]]
name = "deployer"
state = "external"
[[ownership]]
kind = "database"
name = "testdb"
owner = "active"
[[ownership]]
kind = "schema"
name = "managed"
owner = "active"
`), 0o600))
	_, err = planCmd.GeneratePlan(&planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "managed", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-review-retiring-owner-test", SSLMode: "disable",
	}, sharedEmbeddedPG)
	require.ErrorContains(t, err, `retiring schema execution role "retiring" would own newly created objects`)
}

func TestReviewSessionAdminMembershipRevocationRunsLast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	_, err := admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN NOINHERIT CREATEROLE PASSWORD 'deployer-pass';
SET ROLE deployer;
CREATE ROLE app_group;
CREATE ROLE z_user;
RESET ROLE;
`)
	require.NoError(t, err)
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)
	if majorVersion >= 16 {
		t.Skip("session-granted self ADMIN memberships are only constructible before PostgreSQL 16")
	}
	grantAdmin := "GRANT app_group TO deployer WITH ADMIN OPTION"
	grantUser := "GRANT app_group TO z_user"
	_, err = admin.ExecContext(ctx, "SET ROLE deployer; "+grantAdmin+"; "+grantUser+"; RESET ROLE")
	require.NoError(t, err)
	dir := t.TempDir()
	schemaFile := filepath.Join(dir, "schema.sql")
	require.NoError(t, os.WriteFile(schemaFile, nil, 0o600))
	globalFile := filepath.Join(dir, "global.toml")
	require.NoError(t, os.WriteFile(globalFile, []byte(`
version = 1
[[roles]]
name = "app_group"
state = "external"
[[roles]]
name = "deployer"
state = "external"
[[roles]]
name = "z_user"
state = "external"
[[memberships]]
role = "app_group"
member = "deployer"
state = "absent"
[[memberships]]
role = "app_group"
member = "z_user"
state = "absent"
`), 0o600))
	config := &planCmd.PlanConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "public", File: schemaFile, GlobalFile: globalFile,
		ApplicationName: "pgschema-review-admin-revoke-test", SSLMode: "disable",
	}
	migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
	require.NoError(t, err)
	require.NoError(t, ApplyMigration(&ApplyConfig{
		Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
		Schema: "public", Plan: migrationPlan, AutoApprove: true, Quiet: true,
		ApplicationName: "pgschema-review-admin-revoke-test", SSLMode: "disable",
	}, nil))
	var memberships int
	require.NoError(t, admin.QueryRowContext(ctx, `
SELECT count(*) FROM pg_auth_members m
JOIN pg_roles r ON r.oid = m.roleid
JOIN pg_roles u ON u.oid = m.member
WHERE r.rolname = 'app_group' AND u.rolname IN ('deployer', 'z_user')`).Scan(&memberships))
	require.Zero(t, memberships)
}

func TestReviewOwnershipTransferUsesOldOwnerAuthority(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)

	_, err = admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN NOINHERIT CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE old_owner;
CREATE ROLE new_owner;
GRANT CREATE ON SCHEMA public TO old_owner;
CREATE TABLE public.documents (id bigint PRIMARY KEY);
ALTER TABLE public.documents OWNER TO old_owner;
`)
	require.NoError(t, err)
	grantOldOwner := "GRANT old_owner TO deployer"
	grantNewOwner := "GRANT new_owner TO deployer WITH ADMIN OPTION"
	if majorVersion >= 16 {
		grantOldOwner += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
		grantNewOwner = "GRANT new_owner TO deployer WITH ADMIN TRUE, INHERIT FALSE, SET TRUE"
	}
	_, err = admin.ExecContext(ctx, fmt.Sprintf("%s;\n%s;", grantOldOwner, grantNewOwner))
	require.NoError(t, err)
	deployer, err := util.Connect(&util.ConnectionConfig{
		Host: host, Port: port, Database: database, User: "deployer", Password: "deployer-pass",
		SSLMode: "disable", ApplicationName: "pgschema-review-owner-transfer-test",
	})
	require.NoError(t, err)
	defer deployer.Close()

	selection := globalstate.Selection{
		Roles:     []string{"deployer", "old_owner", "new_owner"},
		Ownership: []globalstate.Ownership{{Kind: "table", Name: "public.documents", Owner: "new_owner"}},
	}
	snapshot, err := globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	manifest := globalstate.Manifest{
		Version: 1,
		Roles: []globalstate.Role{
			{Name: "deployer", State: globalstate.StateExternal},
			{Name: "old_owner", State: globalstate.StateExternal},
			{Name: "new_owner", State: globalstate.StateExternal},
		},
		Memberships: []globalstate.Membership{{
			Role: "new_owner", Member: "old_owner", State: globalstate.StatePresent,
			Admin: false, Inherit: majorVersion < 16, Set: true,
		}},
		Ownership: selection.Ownership,
	}
	_, err = globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.ErrorContains(t, err, `new owner role "new_owner" lacks required CREATE authority`)

	_, err = admin.ExecContext(ctx, `GRANT CREATE ON SCHEMA public TO new_owner`)
	require.NoError(t, err)
	snapshot, err = globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	changes, err := globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.NoError(t, err)
	require.Len(t, changes, 2)
	for _, change := range changes {
		_, err = deployer.ExecContext(ctx, change.SQL)
		require.NoError(t, err)
	}
}

func TestReviewPostgres16MultiGrantorMembershipFailsBeforeMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, _, _, _, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)
	if majorVersion < 16 {
		t.Skip("multiple membership grantors require PostgreSQL 16 or newer")
	}

	_, err = admin.ExecContext(ctx, `
CREATE ROLE app_group;
CREATE ROLE app_login;
CREATE ROLE grantor_one;
CREATE ROLE grantor_two;
GRANT app_group TO grantor_one, grantor_two WITH ADMIN OPTION;
SET ROLE grantor_one;
GRANT app_group TO app_login WITH INHERIT TRUE, SET TRUE;
RESET ROLE;
SET ROLE grantor_two;
GRANT app_group TO app_login WITH INHERIT TRUE, SET TRUE;
RESET ROLE;
`)
	require.NoError(t, err)
	selection := globalstate.Selection{
		Roles:       []string{"app_group", "app_login"},
		Memberships: []globalstate.MembershipRef{{Role: "app_group", Member: "app_login"}},
	}
	snapshot, err := globalstate.Inspect(ctx, admin, selection, majorVersion)
	require.NoError(t, err)
	manifest := globalstate.Manifest{
		Version: 1,
		Roles: []globalstate.Role{
			{Name: "app_group", State: globalstate.StateExternal},
			{Name: "app_login", State: globalstate.StateExternal},
		},
		Memberships: []globalstate.Membership{{
			Role: "app_group", Member: "app_login", State: globalstate.StatePresent,
			Admin: false, Inherit: false, Set: true,
		}},
	}

	_, err = globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.ErrorContains(t, err, "multiple grantors")

	_, err = admin.ExecContext(ctx, `REVOKE app_group FROM app_login GRANTED BY grantor_two`)
	require.NoError(t, err)
	snapshot, err = globalstate.Inspect(ctx, admin, selection, majorVersion)
	require.NoError(t, err)
	manifest.Memberships[0].State = globalstate.StateAbsent
	_, err = globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.ErrorContains(t, err, `foreign grantor "grantor_one"`)
}

func TestReviewDatabaseOwnershipTransferValidatesEffectiveExecutor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()
	target := testutil.SetupPostgres(t)
	defer target.Stop()
	admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
	defer admin.Close()
	majorVersion, err := detectPostgresMajorVersion(admin)
	require.NoError(t, err)

	_, err = admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN CREATEDB CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE old_owner;
CREATE ROLE new_owner CREATEDB;
`)
	require.NoError(t, err)
	grantOldOwner := "GRANT old_owner TO deployer"
	grantNewOwner := "GRANT new_owner TO deployer WITH ADMIN OPTION"
	grantTransition := "GRANT new_owner TO old_owner"
	if majorVersion >= 16 {
		grantOldOwner += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
		grantNewOwner = "GRANT new_owner TO deployer WITH ADMIN TRUE, INHERIT FALSE, SET TRUE"
		grantTransition += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
	}
	_, err = admin.ExecContext(ctx, fmt.Sprintf("%s;\n%s;\n%s;", grantOldOwner, grantNewOwner, grantTransition))
	require.NoError(t, err)
	_, err = admin.ExecContext(ctx, `CREATE DATABASE review_owned_db OWNER old_owner`)
	require.NoError(t, err)

	deployer, err := util.Connect(&util.ConnectionConfig{
		Host: host, Port: port, Database: database, User: "deployer", Password: "deployer-pass",
		SSLMode: "disable", ApplicationName: "pgschema-review-database-owner-test",
	})
	require.NoError(t, err)
	defer deployer.Close()
	selection := globalstate.Selection{
		Roles: []string{"deployer", "old_owner", "new_owner"},
		Ownership: []globalstate.Ownership{{
			Kind: "database", Name: "review_owned_db", Owner: "new_owner",
		}},
	}
	manifest := globalstate.Manifest{
		Version: 1,
		Roles: []globalstate.Role{
			{Name: "deployer", State: globalstate.StateExternal},
			{Name: "old_owner", State: globalstate.StateExternal},
			{Name: "new_owner", State: globalstate.StateExternal},
		},
		Ownership: selection.Ownership,
	}
	snapshot, err := globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	_, err = globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.ErrorContains(t, err, `current owner role "old_owner" lacks CREATEDB`)

	_, err = admin.ExecContext(ctx, `ALTER ROLE old_owner CREATEDB`)
	require.NoError(t, err)
	snapshot, err = globalstate.Inspect(ctx, deployer, selection, majorVersion)
	require.NoError(t, err)
	changes, err := globalstate.PlanChanges(manifest, snapshot, majorVersion)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.Contains(t, changes[0].SQL, `SET ROLE old_owner; ALTER DATABASE review_owned_db OWNER TO new_owner; RESET ROLE`)
}

func TestReviewSchemaOwnershipTransferUsesEffectiveExecutorCreateAuthority(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	tests := []struct {
		name              string
		createRole        string
		wantPlanningError bool
	}{
		{name: "current owner has CREATE", createRole: "old_owner"},
		{name: "only new owner has CREATE", createRole: "new_owner", wantPlanningError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			target := testutil.SetupPostgres(t)
			defer target.Stop()
			admin, host, port, database, _, _ := testutil.ConnectToPostgres(t, target)
			defer admin.Close()
			majorVersion, err := detectPostgresMajorVersion(admin)
			require.NoError(t, err)

			_, err = admin.ExecContext(ctx, `
CREATE ROLE deployer LOGIN NOINHERIT CREATEROLE PASSWORD 'deployer-pass';
CREATE ROLE old_owner NOINHERIT;
CREATE ROLE new_owner NOINHERIT;
CREATE SCHEMA managed AUTHORIZATION old_owner;
CREATE TABLE managed.documents (id bigint PRIMARY KEY);
ALTER TABLE managed.documents OWNER TO old_owner;
`)
			require.NoError(t, err)
			_, err = admin.ExecContext(ctx, fmt.Sprintf(
				"REVOKE CREATE ON DATABASE %s FROM PUBLIC, old_owner, new_owner; GRANT CREATE ON DATABASE %s TO %s",
				ir.QuoteIdentifier(database), ir.QuoteIdentifier(database), ir.QuoteIdentifier(tt.createRole),
			))
			require.NoError(t, err)
			grantOldOwner := "GRANT old_owner TO deployer"
			grantNewOwner := "GRANT new_owner TO deployer"
			grantTransition := "GRANT new_owner TO old_owner"
			if majorVersion >= 16 {
				grantOldOwner += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
				grantNewOwner += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
				grantTransition += " WITH ADMIN FALSE, INHERIT FALSE, SET TRUE"
			}
			_, err = admin.ExecContext(ctx, fmt.Sprintf("%s;\n%s;\n%s;", grantOldOwner, grantNewOwner, grantTransition))
			require.NoError(t, err)

			dir := t.TempDir()
			schemaFile := filepath.Join(dir, "schema.sql")
			require.NoError(t, os.WriteFile(schemaFile, []byte("CREATE TABLE documents (id bigint PRIMARY KEY);\n"), 0o600))
			globalFile := filepath.Join(dir, "global.toml")
			membershipOptions := ""
			if majorVersion >= 16 {
				membershipOptions = "inherit = false\nset = true\n"
			}
			require.NoError(t, os.WriteFile(globalFile, []byte(fmt.Sprintf(`
version = 1

[[roles]]
name = "deployer"
state = "external"

[[roles]]
name = "old_owner"
state = "external"

[[roles]]
name = "new_owner"
state = "external"

[[memberships]]
role = "old_owner"
member = "deployer"
%s
[[memberships]]
role = "new_owner"
member = "deployer"
%s
[[memberships]]
role = "new_owner"
member = "old_owner"
%s
[[ownership]]
kind = "schema"
name = "managed"
owner = "new_owner"
`, membershipOptions, membershipOptions, membershipOptions)), 0o600))
			config := &planCmd.PlanConfig{
				Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
				Schema: "managed", File: schemaFile, GlobalFile: globalFile,
				ApplicationName: "pgschema-review-schema-create-authority-test", SSLMode: "disable",
			}
			migrationPlan, err := planCmd.GeneratePlan(config, sharedEmbeddedPG)
			if tt.wantPlanningError {
				require.ErrorContains(t, err, `current owner role "old_owner" lacks required CREATE authority`)
				var owner string
				require.NoError(t, admin.QueryRowContext(ctx,
					`SELECT pg_catalog.pg_get_userbyid(nspowner) FROM pg_catalog.pg_namespace WHERE nspname = 'managed'`,
				).Scan(&owner))
				require.Equal(t, "old_owner", owner)
				return
			}
			require.NoError(t, err)
			require.NoError(t, ApplyMigration(&ApplyConfig{
				Host: host, Port: port, DB: database, User: "deployer", Password: "deployer-pass",
				Schema: "managed", Plan: migrationPlan, AutoApprove: true, Quiet: true,
				ApplicationName: "pgschema-review-schema-create-authority-test", SSLMode: "disable",
			}, nil))
			var owner string
			require.NoError(t, admin.QueryRowContext(ctx,
				`SELECT pg_catalog.pg_get_userbyid(nspowner) FROM pg_catalog.pg_namespace WHERE nspname = 'managed'`,
			).Scan(&owner))
			require.Equal(t, "new_owner", owner)
		})
	}
}
