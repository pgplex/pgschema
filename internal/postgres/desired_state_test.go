package postgres

import (
	"testing"
)

func TestReplaceSchemaInSearchPath(t *testing.T) {
	tests := []struct {
		name         string
		sql          string
		targetSchema string
		tempSchema   string
		expected     string
	}{
		{
			name:         "unquoted with equals",
			sql:          "SET search_path = public, pg_temp",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     `SET search_path = "pgschema_tmp_20260302_000000_abcd1234", pg_temp`,
		},
		{
			name:         "unquoted with TO",
			sql:          "SET search_path TO public",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     `SET search_path TO "pgschema_tmp_20260302_000000_abcd1234"`,
		},
		{
			name:         "quoted target schema",
			sql:          `SET search_path = "public", pg_temp`,
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     `SET search_path = "pgschema_tmp_20260302_000000_abcd1234", pg_temp`,
		},
		{
			name:         "case insensitive schema match",
			sql:          "SET search_path = PUBLIC, pg_temp",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     `SET search_path = "pgschema_tmp_20260302_000000_abcd1234", pg_temp`,
		},
		{
			name:         "mixed case schema",
			sql:          "SET search_path = Public, pg_temp",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     `SET search_path = "pgschema_tmp_20260302_000000_abcd1234", pg_temp`,
		},
		{
			name:         "schema not in search_path is no-op",
			sql:          "SET search_path = pg_catalog, pg_temp",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_20260302_000000_abcd1234",
			expected:     "SET search_path = pg_catalog, pg_temp",
		},
		{
			name:         "multiple functions in same SQL",
			sql:          "CREATE FUNCTION f1() RETURNS void LANGUAGE sql SET search_path = public AS $$ SELECT 1; $$;\nCREATE FUNCTION f2() RETURNS void LANGUAGE sql SET search_path = public, pg_temp AS $$ SELECT 2; $$;",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     "CREATE FUNCTION f1() RETURNS void LANGUAGE sql SET search_path = \"pgschema_tmp_xxx\" AS $$ SELECT 1; $$;\nCREATE FUNCTION f2() RETURNS void LANGUAGE sql SET search_path = \"pgschema_tmp_xxx\", pg_temp AS $$ SELECT 2; $$;",
		},
		{
			name:         "empty target schema returns unchanged",
			sql:          "SET search_path = public, pg_temp",
			targetSchema: "",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     "SET search_path = public, pg_temp",
		},
		{
			name:         "empty temp schema returns unchanged",
			sql:          "SET search_path = public, pg_temp",
			targetSchema: "public",
			tempSchema:   "",
			expected:     "SET search_path = public, pg_temp",
		},
		{
			name:         "no search_path in SQL is no-op",
			sql:          "CREATE TABLE foo (id int);",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     "CREATE TABLE foo (id int);",
		},
		{
			name:         "non-public target schema",
			sql:          "SET search_path = myschema, public",
			targetSchema: "myschema",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     `SET search_path = "pgschema_tmp_xxx", public`,
		},
		{
			name:         "does not match partial schema names",
			sql:          "SET search_path = public_data, pg_temp",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     "SET search_path = public_data, pg_temp",
		},
		{
			name:         "does not replace quoted schema with different case",
			sql:          `SET search_path = "PUBLIC", pg_temp`,
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     `SET search_path = "PUBLIC", pg_temp`,
		},
		{
			name:         "single-line BEGIN ATOMIC function",
			sql:          "CREATE FUNCTION f1() RETURNS int LANGUAGE sql SET search_path = public BEGIN ATOMIC SELECT 1; END;",
			targetSchema: "public",
			tempSchema:   "pgschema_tmp_xxx",
			expected:     `CREATE FUNCTION f1() RETURNS int LANGUAGE sql SET search_path = "pgschema_tmp_xxx" BEGIN ATOMIC SELECT 1; END;`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceSchemaInSearchPath(tt.sql, tt.targetSchema, tt.tempSchema)
			if result != tt.expected {
				t.Errorf("replaceSchemaInSearchPath() =\n%s\nwant:\n%s", result, tt.expected)
			}
		})
	}
}
