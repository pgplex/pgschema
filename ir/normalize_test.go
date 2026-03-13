package ir

import (
	"testing"
)

func TestStripSchemaPrefixFromBody(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		schema   string
		expected string
	}{
		{
			name:     "empty body",
			body:     "",
			schema:   "public",
			expected: "",
		},
		{
			name:     "empty schema",
			body:     "SELECT * FROM public.users",
			schema:   "",
			expected: "SELECT * FROM public.users",
		},
		{
			name:     "no match",
			body:     "SELECT * FROM users",
			schema:   "public",
			expected: "SELECT * FROM users",
		},
		{
			name:     "simple table reference",
			body:     "SELECT * FROM public.users",
			schema:   "public",
			expected: "SELECT * FROM users",
		},
		{
			name:     "multiple references",
			body:     "INSERT INTO public.users SELECT * FROM public.accounts WHERE public.accounts.id > 0",
			schema:   "public",
			expected: "INSERT INTO users SELECT * FROM accounts WHERE accounts.id > 0",
		},
		{
			name:     "preserves string literal",
			body:     "RETURN 'Table: public.users'",
			schema:   "public",
			expected: "RETURN 'Table: public.users'",
		},
		{
			name:     "preserves escaped quotes in string",
			body:     "RETURN 'it''s public.users here'",
			schema:   "public",
			expected: "RETURN 'it''s public.users here'",
		},
		{
			name:     "strips outside but preserves inside string",
			body:     "SELECT public.users.id, 'public.users' FROM public.users",
			schema:   "public",
			expected: "SELECT users.id, 'public.users' FROM users",
		},
		{
			name:     "does not match partial identifier",
			body:     "SELECT * FROM not_public.users",
			schema:   "public",
			expected: "SELECT * FROM not_public.users",
		},
		{
			name:     "different schema not stripped",
			body:     "SELECT * FROM other_schema.users",
			schema:   "public",
			expected: "SELECT * FROM other_schema.users",
		},
		{
			name:     "type cast with schema",
			body:     "SELECT x::public.my_type FROM public.users",
			schema:   "public",
			expected: "SELECT x::my_type FROM users",
		},
		{
			name:     "start of body",
			body:     "public.users WHERE id = 1",
			schema:   "public",
			expected: "users WHERE id = 1",
		},
		{
			name:     "plpgsql function body",
			body:     "\nBEGIN\n    RETURN (SELECT count(*)::integer FROM public.users);\nEND;\n",
			schema:   "public",
			expected: "\nBEGIN\n    RETURN (SELECT count(*)::integer FROM users);\nEND;\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripSchemaPrefixFromBody(tt.body, tt.schema)
			if result != tt.expected {
				t.Errorf("StripSchemaPrefixFromBody(%q, %q) = %q, want %q", tt.body, tt.schema, result, tt.expected)
			}
		})
	}
}

func TestNormalizeViewStripsSchemaPrefixFromDefinition(t *testing.T) {
	tests := []struct {
		name       string
		schema     string
		definition string
		expected   string
	}{
		{
			name:       "strips same-schema function qualification",
			schema:     "public",
			definition: " SELECT id,\n    created_at\n   FROM categories c\n  WHERE public.nlevel(path) = 8",
			expected:   " SELECT id,\n    created_at\n   FROM categories c\n  WHERE nlevel(path) = 8",
		},
		{
			name:       "preserves cross-schema function qualification",
			schema:     "public",
			definition: " SELECT id\n   FROM t\n  WHERE other_schema.some_func(x) = 1",
			expected:   " SELECT id\n   FROM t\n  WHERE other_schema.some_func(x) = 1",
		},
		{
			name:       "strips same-schema table reference",
			schema:     "public",
			definition: " SELECT id\n   FROM public.categories c\n  WHERE nlevel(path) = 8",
			expected:   " SELECT id\n   FROM categories c\n  WHERE nlevel(path) = 8",
		},
		{
			name:       "no-op when no schema prefix present",
			schema:     "public",
			definition: " SELECT id,\n    created_at\n   FROM categories c\n  WHERE nlevel(path) = 8",
			expected:   " SELECT id,\n    created_at\n   FROM categories c\n  WHERE nlevel(path) = 8",
		},
		{
			name:       "strips multiple occurrences",
			schema:     "myschema",
			definition: " SELECT myschema.func1(x), myschema.func2(y)\n   FROM myschema.tbl",
			expected:   " SELECT func1(x), func2(y)\n   FROM tbl",
		},
		{
			name:       "preserves string literals containing schema prefix",
			schema:     "public",
			definition: " SELECT 'public.data' AS label\n   FROM public.categories",
			expected:   " SELECT 'public.data' AS label\n   FROM categories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := &View{
				Schema:     tt.schema,
				Name:       "test_view",
				Definition: tt.definition,
			}
			normalizeView(view)
			if view.Definition != tt.expected {
				t.Errorf("normalizeView() definition = %q, want %q", view.Definition, tt.expected)
			}
		})
	}
}

func TestNormalizeCheckClause(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "varchar IN with ::text cast - user form (has extra parens around column)",
			input:    "CHECK ((status)::text = ANY (ARRAY['pending'::character varying, 'shipped'::character varying, 'delivered'::character varying]::text[]))",
			expected: "CHECK (status::text IN ('pending'::character varying, 'shipped'::character varying, 'delivered'::character varying))",
		},
		{
			name:     "varchar IN without explicit cast - user form (no extra parens)",
			input:    "CHECK (status::text = ANY (ARRAY['pending'::character varying, 'shipped'::character varying, 'delivered'::character varying]::text[]))",
			expected: "CHECK (status::text IN ('pending'::character varying, 'shipped'::character varying, 'delivered'::character varying))",
		},
		{
			name:     "varchar IN with double cast - applied form (pgschema-generated SQL stored by PostgreSQL)",
			input:    "CHECK (status::text = ANY (ARRAY['pending'::character varying::text, 'shipped'::character varying::text, 'delivered'::character varying::text]))",
			expected: "CHECK (status::text IN ('pending'::character varying, 'shipped'::character varying, 'delivered'::character varying))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeCheckClause(tt.input)
			t.Logf("Input:    %s", tt.input)
			t.Logf("Output:   %s", result)
			t.Logf("Expected: %s", tt.expected)
			if result != tt.expected {
				t.Errorf("normalizeCheckClause() = %v, want %v", result, tt.expected)
			}
		})
	}
}
