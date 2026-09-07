package ir

import "testing"

func TestStripSchemaQualifiers(t *testing.T) {
	cases := []struct{ in, schema, want string }{
		{"r public.v", "public", "r v"},
		{"public.v", "public", "v"},
		{"ORDER BY public.v", "public", "ORDER BY v"},
		{`x integer, "MySchema".v`, "MySchema", "x integer, v"},
		{`"public.foo"`, "public", `"public.foo"`},
		{`public."public.foo"`, "public", `"public.foo"`},
		{"other.v", "public", "other.v"},
		{"not_public.v", "public", "not_public.v"},
		{"numeric(10,2), public.v[]", "public", "numeric(10,2), v[]"},
		{"", "public", ""},
	}
	for _, c := range cases {
		if got := StripSchemaQualifiers(c.in, c.schema); got != c.want {
			t.Errorf("StripSchemaQualifiers(%q, %q) = %q, want %q", c.in, c.schema, got, c.want)
		}
	}
}
