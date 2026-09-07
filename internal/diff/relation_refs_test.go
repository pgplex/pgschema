package diff

import (
	"reflect"
	"testing"
)

func TestRelationReferences(t *testing.T) {
	cases := []struct {
		body string
		want []string
	}{
		{"SELECT count(*) FROM v", []string{"v"}},
		{"SELECT * FROM t, v WHERE t.id = v.id", []string{"t", "v"}},
		{"SELECT * FROM t AS a, v b, w", []string{"t", "v", "w"}},
		{"SELECT * FROM unnest(xs) AS u(x), v", []string{"v"}},
		{"SELECT * FROM (SELECT 1) AS s, v", []string{"v"}},
		{"SELECT * FROM ONLY v", []string{"v"}},
		{"SELECT * FROM ONLY (v)", []string{"v"}},
		{"SELECT * FROM public . v", []string{"public . v"}},
		{`SELECT * FROM "My View"`, []string{`"My View"`}},
		{"SELECT * FROM t JOIN v ON t.id = v.id", []string{"t", "v"}},
		{"SELECT * FROM v WHERE x IN (SELECT y FROM w)", []string{"v", "w"}},
		{"INSERT INTO t(a, b) VALUES (1, 2)", []string{"t"}},
		{"INSERT INTO t (a, b) SELECT a, b FROM v", []string{"t", "v"}},
		{"UPDATE t SET a = 1", []string{"t"}},
		{"DELETE FROM t WHERE id = 1", []string{"t"}},
		{"SELECT * FROM v ORDER BY x, y", []string{"v"}},
		{"SELECT * FROM v GROUP BY a, b", []string{"v"}},
		{"DELETE FROM t USING v WHERE t.id = v.id", []string{"t", "v"}},
		{"MERGE INTO t USING v ON t.id = v.id WHEN MATCHED THEN DELETE", []string{"t", "v"}},
		{"SELECT * FROM t JOIN v USING (id)", []string{"t", "v"}},
		{"SELECT * FROM unnest(xs) WITH ORDINALITY AS u(x, n), v", []string{"v"}},
	}
	for _, c := range cases {
		if got := relationReferences(c.body); !reflect.DeepEqual(got, c.want) {
			t.Errorf("relationReferences(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestStripLeadingIdentifier(t *testing.T) {
	cases := []struct{ in, want string }{
		{"r v", "v"},
		{`"my col" numeric(10,2)`, "numeric(10,2)"},
		{`"r""x" v`, "v"},
		{"v", ""},
	}
	for _, c := range cases {
		if got := stripLeadingIdentifier(c.in); got != c.want {
			t.Errorf("stripLeadingIdentifier(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
