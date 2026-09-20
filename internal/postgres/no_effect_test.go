package postgres

import (
	"reflect"
	"strings"
	"testing"
)

func TestStripNoEffectStatements(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want []NoEffectStatement
		// kept lists fragments that must survive; gone lists fragments that must not.
		kept []string
		gone []string
	}{
		{
			name: "owner to on each object kind",
			sql: `CREATE TABLE item (id int);
ALTER TABLE public.item OWNER TO eval_other;
ALTER VIEW item_view OWNER TO eval_other;
ALTER MATERIALIZED VIEW item_mv OWNER TO eval_other;
ALTER FUNCTION calculate(integer, text) OWNER TO eval_other;
ALTER SEQUENCE counter OWNER TO CURRENT_USER;
ALTER TYPE mood OWNER TO "Mixed Role";`,
			want: []NoEffectStatement{
				{Kind: NoEffectOwner, SQL: `ALTER TABLE public.item OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER VIEW item_view OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER MATERIALIZED VIEW item_mv OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER FUNCTION calculate(integer, text) OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER SEQUENCE counter OWNER TO CURRENT_USER`},
				{Kind: NoEffectOwner, SQL: `ALTER TYPE mood OWNER TO "Mixed Role"`},
			},
			kept: []string{"CREATE TABLE item (id int);"},
			gone: []string{"OWNER TO"},
		},
		{
			name: "global default privileges, schema-scoped form is kept",
			sql: `ALTER DEFAULT PRIVILEGES FOR ROLE eval_owner
  GRANT SELECT ON TABLES TO eval_reader;
ALTER DEFAULT PRIVILEGES FOR ROLE eval_owner IN SCHEMA public GRANT SELECT ON TABLES TO eval_reader;
alter default privileges revoke execute on functions from public;`,
			want: []NoEffectStatement{
				{Kind: NoEffectGlobalDefaultPrivileges, SQL: `ALTER DEFAULT PRIVILEGES FOR ROLE eval_owner GRANT SELECT ON TABLES TO eval_reader`},
				{Kind: NoEffectGlobalDefaultPrivileges, SQL: `alter default privileges revoke execute on functions from public`},
			},
			kept: []string{"IN SCHEMA public GRANT SELECT ON TABLES TO eval_reader;"},
			gone: []string{"eval_owner\n", "revoke execute"},
		},
		{
			name: "identifiers named owner are not the OWNER TO action",
			sql: `ALTER TABLE item RENAME COLUMN owner TO proprietor;
ALTER TABLE item RENAME owner TO proprietor;
ALTER TABLE item RENAME CONSTRAINT owner TO item_owner;
ALTER TYPE pair RENAME ATTRIBUTE owner TO proprietor;
ALTER TABLE "owner" ADD COLUMN "to" int;
ALTER SEQUENCE counter OWNED BY item.id;`,
			want: nil,
			kept: []string{"RENAME COLUMN owner", "RENAME owner", "RENAME CONSTRAINT owner", "RENAME ATTRIBUTE owner", "OWNED BY item.id"},
		},
		{
			name: "table named owner still reports its OWNER TO",
			sql:  `ALTER TABLE owner OWNER TO eval_other;`,
			want: []NoEffectStatement{{Kind: NoEffectOwner, SQL: `ALTER TABLE owner OWNER TO eval_other`}},
			gone: []string{"eval_other"},
		},
		{
			name: "owner to among other actions: only that action is dropped",
			sql: `ALTER TABLE item OWNER TO eval_other, ADD COLUMN note text;
ALTER TABLE item ADD COLUMN a int, OWNER TO eval_other;
ALTER TABLE item ADD COLUMN b numeric(10, 2), OWNER TO eval_other, ADD COLUMN c int;`,
			want: []NoEffectStatement{
				{Kind: NoEffectOwner, SQL: `ALTER TABLE item OWNER TO eval_other, ADD COLUMN note text`, Partial: true},
				{Kind: NoEffectOwner, SQL: `ALTER TABLE item ADD COLUMN a int, OWNER TO eval_other`, Partial: true},
				{Kind: NoEffectOwner, SQL: `ALTER TABLE item ADD COLUMN b numeric(10, 2), OWNER TO eval_other, ADD COLUMN c int`, Partial: true},
			},
			kept: []string{"ADD COLUMN note text;", "ADD COLUMN a int", "ADD COLUMN b numeric(10, 2),", "ADD COLUMN c int;"},
			gone: []string{"eval_other", "int,  "},
		},
		{
			name: "index owner, and two-word kinds need their second keyword",
			sql: `ALTER INDEX item_idx OWNER TO eval_other;
ALTER FOREIGN TABLE remote_item OWNER TO eval_other;
ALTER FOREIGN DATA WRAPPER fdw OWNER TO fdw_owner;
ALTER MATERIALIZED VIEW mv OWNER TO eval_other;`,
			want: []NoEffectStatement{
				{Kind: NoEffectOwner, SQL: `ALTER INDEX item_idx OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER FOREIGN TABLE remote_item OWNER TO eval_other`},
				{Kind: NoEffectOwner, SQL: `ALTER MATERIALIZED VIEW mv OWNER TO eval_other`},
			},
			kept: []string{"ALTER FOREIGN DATA WRAPPER fdw OWNER TO fdw_owner;"},
			gone: []string{"eval_other"},
		},
		{
			name: "E-string with a backslash-escaped quote stays one literal",
			sql:  `COMMENT ON TABLE item IS E'prefix \'; ALTER TABLE victim OWNER TO app_owner; suffix';`,
			want: nil,
			kept: []string{`E'prefix \'; ALTER TABLE victim OWNER TO app_owner; suffix'`},
		},
		{
			name: "trailing comment before the terminator",
			sql:  "ALTER TABLE item OWNER TO eval_other /* later */ ;\nCREATE TABLE t (id int);",
			want: []NoEffectStatement{{Kind: NoEffectOwner, SQL: `ALTER TABLE item OWNER TO eval_other`}},
			kept: []string{"CREATE TABLE t (id int);"},
			gone: []string{"eval_other", ";\nCREATE"},
		},
		{
			name: "literals, comments and dollar-quoted bodies are not scanned",
			sql: `-- ALTER TABLE item OWNER TO commented;
/* ALTER DEFAULT PRIVILEGES GRANT SELECT ON TABLES TO commented; */
COMMENT ON TABLE item IS 'run ALTER TABLE item OWNER TO literal; later';
DO $$ BEGIN
  ALTER TABLE item OWNER TO in_body;
END $$;`,
			want: nil,
			kept: []string{"OWNER TO commented", "OWNER TO literal", "OWNER TO in_body"},
		},
		{
			name: "comment inside the statement and a leading comment",
			sql: `-- hand ownership to the app role
ALTER TABLE item /* why */ OWNER TO eval_other;
CREATE INDEX idx ON item (id);`,
			want: []NoEffectStatement{{Kind: NoEffectOwner, SQL: `ALTER TABLE item OWNER TO eval_other`}},
			kept: []string{"-- hand ownership to the app role", "CREATE INDEX idx ON item (id);"},
			gone: []string{"eval_other"},
		},
		{
			name: "objects outside the schema are left alone",
			sql: `ALTER SCHEMA app OWNER TO eval_other;
ALTER DATABASE app OWNER TO eval_other;`,
			want: nil,
			kept: []string{"ALTER SCHEMA app OWNER TO", "ALTER DATABASE app OWNER TO"},
		},
		{
			name: "semicolon inside a quoted identifier does not split",
			sql:  `ALTER TABLE "odd;name" OWNER TO eval_other;`,
			want: []NoEffectStatement{{Kind: NoEffectOwner, SQL: `ALTER TABLE "odd;name" OWNER TO eval_other`}},
			gone: []string{"odd;name"},
		},
		{
			name: "last statement without a terminator",
			sql:  "CREATE TABLE item (id int);\nALTER TABLE item OWNER TO eval_other",
			want: []NoEffectStatement{{Kind: NoEffectOwner, SQL: `ALTER TABLE item OWNER TO eval_other`}},
			gone: []string{"OWNER TO"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := StripNoEffectStatements(tt.sql)
			if !reflect.DeepEqual(found, tt.want) {
				t.Errorf("found = %+v\nwant    %+v", found, tt.want)
			}
			for _, s := range tt.kept {
				if !strings.Contains(got, s) {
					t.Errorf("stripped SQL lost %q:\n%s", s, got)
				}
			}
			for _, s := range tt.gone {
				if strings.Contains(got, s) {
					t.Errorf("stripped SQL still has %q:\n%s", s, got)
				}
			}
			// Removal blanks in place, so PostgreSQL error positions and line
			// numbers still point into the user's file.
			if len(got) != len(tt.sql) || strings.Count(got, "\n") != strings.Count(tt.sql, "\n") {
				t.Errorf("stripped SQL changed shape: len %d -> %d", len(tt.sql), len(got))
			}
		})
	}
}
