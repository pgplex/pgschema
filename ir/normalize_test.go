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

func TestSplitColumnNameAndType(t *testing.T) {
	tests := []struct {
		name         string
		colDef       string
		expectedName string
		expectedType string
	}{
		{"simple", "id integer", "id", "integer"},
		{"schema qualified type", "col public.mytype", "col", "public.mytype"},
		{"quoted identifier", `"full name" text`, `"full name"`, "text"},
		{"quoted with schema type", `"my col" public.mytype`, `"my col"`, "public.mytype"},
		{"quoted with escaped quotes", `"it""s" integer`, `"it""s"`, "integer"},
		{"name only", "id", "id", ""},
		{"empty", "", "", ""},
		{"multi-word type", "col character varying", "col", "character varying"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, typePart := splitColumnNameAndType(tt.colDef)
			if name != tt.expectedName || typePart != tt.expectedType {
				t.Errorf("splitColumnNameAndType(%q) = (%q, %q), want (%q, %q)",
					tt.colDef, name, typePart, tt.expectedName, tt.expectedType)
			}
		})
	}
}

func TestSplitTableColumns(t *testing.T) {
	tests := []struct {
		name     string
		inner    string
		expected []string
	}{
		{
			name:     "simple columns",
			inner:    "id integer, name varchar",
			expected: []string{"id integer", " name varchar"},
		},
		{
			name:     "numeric with precision and scale",
			inner:    "id integer, amount numeric(10, 2), name varchar",
			expected: []string{"id integer", " amount numeric(10, 2)", " name varchar"},
		},
		{
			name:     "nested parentheses",
			inner:    "id integer, val numeric(10, 2), label character varying(100)",
			expected: []string{"id integer", " val numeric(10, 2)", " label character varying(100)"},
		},
		{
			name:     "single column",
			inner:    "id integer",
			expected: []string{"id integer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitTableColumns(tt.inner)
			if len(result) != len(tt.expected) {
				t.Fatalf("splitTableColumns(%q) returned %d parts, want %d: %v", tt.inner, len(result), len(tt.expected), result)
			}
			for i, part := range result {
				if part != tt.expected[i] {
					t.Errorf("splitTableColumns(%q)[%d] = %q, want %q", tt.inner, i, part, tt.expected[i])
				}
			}
		})
	}
}

func TestStripSchemaFromReturnType(t *testing.T) {
	tests := []struct {
		name       string
		returnType string
		schema     string
		expected   string
	}{
		{
			name:       "empty",
			returnType: "",
			schema:     "public",
			expected:   "",
		},
		{
			name:       "simple type no prefix",
			returnType: "integer",
			schema:     "public",
			expected:   "integer",
		},
		{
			name:       "simple type with prefix",
			returnType: "public.mytype",
			schema:     "public",
			expected:   "mytype",
		},
		{
			name:       "SETOF with prefix",
			returnType: "SETOF public.actor",
			schema:     "public",
			expected:   "SETOF actor",
		},
		{
			name:       "TABLE with custom type prefix",
			returnType: "TABLE(id uuid, name varchar, created_at public.datetimeoffset)",
			schema:     "public",
			expected:   "TABLE(id uuid, name varchar, created_at datetimeoffset)",
		},
		{
			name:       "TABLE with multiple custom type prefixes",
			returnType: "TABLE(id uuid, created_at public.datetimeoffset, updated_at public.datetimeoffset)",
			schema:     "public",
			expected:   "TABLE(id uuid, created_at datetimeoffset, updated_at datetimeoffset)",
		},
		{
			name:       "TABLE with no prefix to strip",
			returnType: "TABLE(id uuid, name varchar)",
			schema:     "public",
			expected:   "TABLE(id uuid, name varchar)",
		},
		{
			name:       "TABLE with numeric precision (commas in parens)",
			returnType: "TABLE(id integer, amount numeric(10, 2), name public.mytype)",
			schema:     "public",
			expected:   "TABLE(id integer, amount numeric(10, 2), name mytype)",
		},
		{
			name:       "array type with prefix",
			returnType: "public.mytype[]",
			schema:     "public",
			expected:   "mytype[]",
		},
		{
			name:       "TABLE with quoted column name",
			returnType: `TABLE("full name" public.mytype, id uuid)`,
			schema:     "public",
			expected:   `TABLE("full name" mytype, id uuid)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripSchemaFromReturnType(tt.returnType, tt.schema)
			if result != tt.expected {
				t.Errorf("stripSchemaFromReturnType(%q, %q) = %q, want %q", tt.returnType, tt.schema, result, tt.expected)
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
