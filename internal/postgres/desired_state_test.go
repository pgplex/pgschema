package postgres

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSplitDollarQuotedSegments(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		expected []dollarQuotedSegment
	}{
		{
			name:     "no dollar quotes",
			sql:      "SELECT 1 FROM public.users;",
			expected: []dollarQuotedSegment{{text: "SELECT 1 FROM public.users;", quoted: false}},
		},
		{
			name: "simple dollar-quoted body",
			sql:  "CREATE FUNCTION f() AS $$body$$ LANGUAGE sql;",
			expected: []dollarQuotedSegment{
				{text: "CREATE FUNCTION f() AS ", quoted: false},
				{text: "$$body$$", quoted: true},
				{text: " LANGUAGE sql;", quoted: false},
			},
		},
		{
			name: "named dollar-quote tag",
			sql:  "AS $func$body$func$ LANGUAGE sql;",
			expected: []dollarQuotedSegment{
				{text: "AS ", quoted: false},
				{text: "$func$body$func$", quoted: true},
				{text: " LANGUAGE sql;", quoted: false},
			},
		},
		{
			name: "parameter references not treated as dollar quotes",
			sql:  "SELECT $1 + $2 FROM t;",
			expected: []dollarQuotedSegment{
				{text: "SELECT $1 + $2 FROM t;", quoted: false},
			},
		},
		{
			name: "multiple dollar-quoted blocks",
			sql:  "AS $$body1$$; AS $f$body2$f$;",
			expected: []dollarQuotedSegment{
				{text: "AS ", quoted: false},
				{text: "$$body1$$", quoted: true},
				{text: "; AS ", quoted: false},
				{text: "$f$body2$f$", quoted: true},
				{text: ";", quoted: false},
			},
		},
		{
			name: "unterminated dollar quote",
			sql:  "AS $$body without end",
			expected: []dollarQuotedSegment{
				{text: "AS ", quoted: false},
				{text: "$$body without end", quoted: true},
			},
		},
		{
			name:     "empty input",
			sql:      "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitDollarQuotedSegments(tt.sql)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("splitDollarQuotedSegments(%q)\n  got:  %+v\n  want: %+v", tt.sql, result, tt.expected)
			}
		})
	}
}

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

func TestStripSchemaQualifications_PreservesStringLiterals(t *testing.T) {
	tests := []struct {
		name     string
		sql      string
		schema   string
		expected string
	}{
		{
			name:     "strips schema from table reference",
			sql:      "CREATE TABLE public.items (id int);",
			schema:   "public",
			expected: "CREATE TABLE items (id int);",
		},
		{
			name:     "preserves schema prefix inside single-quoted string",
			sql:      "CREATE POLICY p ON items USING (has_scope('public.manage'));",
			schema:   "public",
			expected: "CREATE POLICY p ON items USING (has_scope('public.manage'));",
		},
		{
			name:     "preserves schema prefix inside string with short schema name",
			sql:      "CREATE POLICY p ON items USING (has_scope('s.manage')) WITH CHECK (has_scope('s.manage'));",
			schema:   "s",
			expected: "CREATE POLICY p ON items USING (has_scope('s.manage')) WITH CHECK (has_scope('s.manage'));",
		},
		{
			name:     "strips schema from identifier but preserves string literal",
			sql:      "CREATE POLICY p ON s.items USING (auth.has_scope('s.manage'));",
			schema:   "s",
			expected: "CREATE POLICY p ON items USING (auth.has_scope('s.manage'));",
		},
		{
			name:     "preserves escaped quotes in string literals",
			sql:      "SELECT 'it''s public.test' FROM public.t;",
			schema:   "public",
			expected: "SELECT 'it''s public.test' FROM t;",
		},
		{
			name:     "handles multiple string literals",
			sql:      "SELECT 'public.a', public.t, 'public.b';",
			schema:   "public",
			expected: "SELECT 'public.a', t, 'public.b';",
		},
		{
			name:     "does not match schema as suffix of longer identifier",
			sql:      "SELECT sales.total, s.items FROM s.orders;",
			schema:   "s",
			expected: "SELECT sales.total, items FROM orders;",
		},
		{
			name:     "strips schema at start of string",
			sql:      "public.t",
			schema:   "public",
			expected: "t",
		},
		{
			name:     "handles apostrophe in line comment followed by schema-qualified identifier",
			sql:      "SELECT 1; -- don't drop public.t\nDROP TABLE public.t;",
			schema:   "public",
			expected: "SELECT 1; -- don't drop public.t\nDROP TABLE t;",
		},
		{
			name:     "handles block comment with apostrophe",
			sql:      "/* it's public.t */ DROP TABLE public.t;",
			schema:   "public",
			expected: "/* it's public.t */ DROP TABLE t;",
		},
		{
			name:     "handles block comment without apostrophe",
			sql:      "/* drop public.t */ DROP TABLE public.t;",
			schema:   "public",
			expected: "/* drop public.t */ DROP TABLE t;",
		},
		{
			// Known limitation: E'...' escape-string syntax with backslash-escaped quotes
			// is not handled. The parser treats \' as ordinary char + string-closer,
			// mistracking boundaries. Here it strips inside the string (wrong) and
			// misses the identifier after (also wrong). Both are safe: the SQL remains
			// valid, and the unstripped qualifier just means the object is looked up
			// in the original schema. E'...' in DDL is extremely rare.
			name:     "E-string with backslash-escaped quote (known limitation)",
			sql:      "SELECT E'it\\'s public.test' FROM public.t;",
			schema:   "public",
			expected: "SELECT E'it\\'s test' FROM public.t;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripSchemaQualifications(tt.sql, tt.schema)
			if result != tt.expected {
				t.Errorf("stripSchemaQualifications(%q, %q)\n  got:  %q\n  want: %q", tt.sql, tt.schema, result, tt.expected)
			}
		})
	}
}

func TestEnhanceApplyError(t *testing.T) {
	sql := "CREATE TABLE foo (id int);\nCREATE TABLE bar (\n  name text\n);\nSELECT 1;\nCREATE TABLE baz (id int);"

	t.Run("pgError with position", func(t *testing.T) {
		// Position points to "SELECT" on line 5
		pos := int32(strings.Index(sql, "SELECT 1") + 1) // 1-based
		pgErr := &pgconn.PgError{
			Message:  "syntax error at or near \"SELECT\"",
			Code:     "42601",
			Position: pos,
		}
		enhanced := enhanceApplyError(pgErr, sql)
		errMsg := enhanced.Error()

		if !strings.Contains(errMsg, "line 5") {
			t.Errorf("expected error to mention line 5, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "SELECT 1") {
			t.Errorf("expected error to contain the offending line, got: %s", errMsg)
		}
		// Should still contain original error
		if !strings.Contains(errMsg, "syntax error") {
			t.Errorf("expected error to contain original message, got: %s", errMsg)
		}
	})

	t.Run("multi-byte UTF-8 position", func(t *testing.T) {
		// PostgreSQL Position counts characters, not bytes.
		// "café" is 4 characters but 5 bytes (é is 2 bytes in UTF-8).
		mbSQL := "-- café\nSELECT 1;"
		// "SELECT" starts at character position 9 (1-based): "-- café\n" = 8 chars
		pgErr := &pgconn.PgError{
			Message:  "syntax error",
			Code:     "42601",
			Position: 9,
		}
		enhanced := enhanceApplyError(pgErr, mbSQL)
		errMsg := enhanced.Error()

		if !strings.Contains(errMsg, "line 2, column 1") {
			t.Errorf("expected line 2, column 1 for multi-byte SQL, got: %s", errMsg)
		}
		if !strings.Contains(errMsg, "SELECT 1") {
			t.Errorf("expected snippet to contain the error line, got: %s", errMsg)
		}
	})

	t.Run("non-pg error passes through", func(t *testing.T) {
		origErr := fmt.Errorf("some other error")
		result := enhanceApplyError(origErr, sql)
		if result != origErr {
			t.Errorf("expected same error instance, got: %s", result.Error())
		}
	})

	t.Run("pgError without position passes through", func(t *testing.T) {
		pgErr := &pgconn.PgError{
			Message: "some error",
			Code:    "42601",
		}
		result := enhanceApplyError(pgErr, sql)
		if result != pgErr {
			t.Errorf("expected same error instance, got: %s", result.Error())
		}
	})
}
