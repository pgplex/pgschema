package postgres

import (
	"reflect"
	"testing"
)

func TestExtractReferencedRoles(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []string
	}{
		{
			name: "object grant with grant option",
			sql:  `GRANT SELECT, INSERT ON TABLE users TO app_user WITH GRANT OPTION;`,
			want: []string{"app_user"},
		},
		{
			name: "multiple grantees and column grant",
			sql: `GRANT SELECT ON users TO reader, writer;
			      GRANT UPDATE (email, to_date) ON TABLE users TO editor;`,
			want: []string{"reader", "writer", "editor"},
		},
		{
			name: "revoke and revoke grant option",
			sql: `REVOKE INSERT ON TABLE users FROM app_user;
			      REVOKE GRANT OPTION FOR SELECT ON TABLE users FROM manager;`,
			want: []string{"app_user", "manager"},
		},
		{
			name: "pseudo roles and predefined roles are skipped",
			sql: `GRANT SELECT ON users TO PUBLIC;
			      GRANT SELECT ON users TO CURRENT_USER, pg_read_all_data, session_user;`,
			want: nil,
		},
		{
			name: "quoted identifiers keep case, unquoted are folded",
			sql:  `GRANT SELECT ON users TO "Mixed Case", UPPER_ROLE;`,
			want: []string{"Mixed Case", "upper_role"},
		},
		{
			name: "policy role lists",
			sql: `CREATE POLICY p ON users FOR SELECT TO app_user, auditor USING (owner = current_user);
			      ALTER POLICY p ON users TO tenant;`,
			want: []string{"app_user", "auditor", "tenant"},
		},
		{
			name: "default privileges grantor and grantee",
			sql:  `ALTER DEFAULT PRIVILEGES FOR ROLE owner_role IN SCHEMA public GRANT SELECT ON TABLES TO reader;`,
			want: []string{"owner_role", "reader"},
		},
		{
			name: "legacy GROUP keyword",
			sql:  `GRANT SELECT ON users TO GROUP admins;`,
			want: []string{"admins"},
		},
		{
			name: "strings, comments and dollar-quoted bodies are ignored",
			sql: `-- GRANT SELECT ON users TO commented_out;
			      DO $$ BEGIN CREATE ROLE in_do_block; GRANT SELECT ON users TO in_do_block; END $$;
			      INSERT INTO log VALUES ('GRANT SELECT ON users TO in_string');
			      GRANT SELECT ON users TO real_role;`,
			want: []string{"real_role"},
		},
		{
			name: "duplicates collapse",
			sql: `GRANT SELECT ON a TO app_user;
			      GRANT SELECT ON b TO app_user;`,
			want: []string{"app_user"},
		},
		{
			name: "comments inside a statement do not split it",
			sql: `GRANT SELECT ON users TO /* read only */ app_user;
			      GRANT INSERT ON users -- staff only
			      TO staff;`,
			want: []string{"app_user", "staff"},
		},
		{
			name: "no roles",
			sql:  `CREATE TABLE grants (id int, policy text, to_date date);`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractReferencedRoles(tt.sql)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractReferencedRoles() = %v, want %v", got, tt.want)
			}
		})
	}
}
