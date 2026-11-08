package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// A dump has no apply role, so it cannot know whether a default privilege
// declared FOR ROLE app_owner will fire when it is restored. It must therefore
// keep every explicit grant (restoring as another role would otherwise lose
// it) and keep the conservative REVOKE for a table whose covered grant was
// withheld (restoring as app_owner would otherwise grant it back). Issue #696.
func TestDumpKeepsExplicitGrantsAndWithheldDefaults(t *testing.T) {
	table := func(name string) *ir.Table {
		return &ir.Table{
			Schema: "public", Name: name,
			Columns: []*ir.Column{{Name: "id", Position: 1, DataType: "integer"}},
		}
	}
	schema := ir.NewIR()
	schema.Schemas["public"] = &ir.Schema{
		Name:   "public",
		Tables: map[string]*ir.Table{"accounts": table("accounts"), "audit_log": table("audit_log")},
		DefaultPrivileges: []*ir.DefaultPrivilege{{
			OwnerRole: "app_owner", ObjectType: ir.DefaultPrivilegeObjectTypeTables,
			Grantee: "app_reader", Privileges: []string{"SELECT"},
		}},
		Privileges: []*ir.Privilege{{
			ObjectType: ir.PrivilegeObjectTypeTable, ObjectName: "accounts",
			Grantee: "app_reader", Privileges: []string{"SELECT"},
		}},
	}

	diffs, err := GenerateDump(schema, "public", false)
	if err != nil {
		t.Fatal(err)
	}
	var statements []string
	for _, d := range diffs {
		for _, s := range d.Statements {
			statements = append(statements, s.SQL)
		}
	}
	sql := strings.Join(statements, "\n")
	for _, want := range []string{
		"ALTER DEFAULT PRIVILEGES FOR ROLE app_owner IN SCHEMA public GRANT SELECT ON TABLES TO app_reader;",
		"GRANT SELECT ON TABLE accounts TO app_reader;",
		"REVOKE SELECT ON TABLE audit_log FROM app_reader;",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("dump is missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "REVOKE SELECT ON TABLE accounts") {
		t.Errorf("dump revokes the explicitly granted privilege:\n%s", sql)
	}
}
