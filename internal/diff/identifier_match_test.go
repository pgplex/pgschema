package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestContainsIdentifier(t *testing.T) {
	cases := []struct {
		text, ident string
		want        bool
	}{
		{"SELECT id FROM users", "users", true},
		{"SELECT id FROM public.users", "users", true},
		{"SELECT id FROM public.users", "public.users", true},
		{"SELECT id FROM other.public.users", "public.users", false},
		{`SELECT id FROM "Users"`, "Users", true},
		{`SELECT id FROM "a""b"`, `a"b`, true},
		{`SELECT id FROM "a"b"`, `a"b`, false},
		{`SELECT id FROM "my schema"."a""b"`, `my schema.a"b`, true},
		{"SELECT id FROM foobar", "foo", false},
		{"SELECT id FROM foo_bar", "foo", false},
		{"SELECT id FROM foo$bar", "foo", false},
		{"SELECT 'users' FROM t", "users", false}, // string literal
		{"SELECT (row)::users FROM t", "users", true},
		{"", "users", false},
	}
	for _, c := range cases {
		if got := containsIdentifier(c.text, c.ident); got != c.want {
			t.Errorf("containsIdentifier(%q, %q) = %v, want %v", c.text, c.ident, got, c.want)
		}
	}
}

func TestViewDependsOnTable_QuotedNames(t *testing.T) {
	view := &ir.View{Schema: "public", Name: "v", Definition: ` SELECT c FROM "a""b";`}
	if !viewDependsOnTable(view, "public", `a"b`) {
		t.Errorf("expected view to depend on table a\"b")
	}
	qualified := &ir.View{Schema: "public", Name: "v", Definition: ` SELECT c FROM "my schema"."Orders";`}
	if !viewDependsOnTable(qualified, "my schema", "Orders") {
		t.Errorf("expected view to depend on \"my schema\".\"Orders\"")
	}
	if viewDependsOnTable(qualified, "my schema", "Order") {
		t.Errorf("did not expect a prefix match")
	}
}

func TestExprReferencesAnyColumn(t *testing.T) {
	cols := map[string]bool{"b": true, "my col": true, `a"b`: true}
	cases := []struct {
		expr string
		want bool
	}{
		{"(b + 1)", true},
		{"b", true},
		{"(t.b > 0)", true},
		{"(\"my col\" * 2)", true},
		{"(b > 10)", true},
		{"(bb + 1)", false},
		{"(a + 1)", false},
		{"b(a)", false},        // function call, not a column
		{"('b'::text)", false}, // string literal
		{`("a""b" + 1)`, true}, // embedded quote doubled by pg_get_expr
		{`("a"b" + 1)`, false},
		{"(b$x + 1)", false}, // $ is part of the identifier
		{"(x$b + 1)", false},
		{"((a)::b)", false}, // type cast, not a column
		{"", false},
	}
	for _, c := range cases {
		if got := exprReferencesAnyColumn(c.expr, cols); got != c.want {
			t.Errorf("exprReferencesAnyColumn(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}
