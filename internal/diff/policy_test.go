package diff

import "testing"

func TestEnsureParentheses(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		{"", ""},
		{"true", "(true)"},
		{"is_admin()", "(is_admin())"},
		{"(is_admin())", "(is_admin())"},
		{"((a > 0) OR (b > 0))", "((a > 0) OR (b > 0))"},
		{"(a > 0) OR (b > 0)", "((a > 0) OR (b > 0))"},
		{"( SELECT is_admin() AS is_admin)", "(( SELECT is_admin() AS is_admin))"},
		{"(SELECT (NOT is_admin()))", "((SELECT (NOT is_admin())))"},
		{"( WITH x AS (SELECT true) SELECT * FROM x)", "(( WITH x AS (SELECT true) SELECT * FROM x))"},
		{"(with_flag)", "(with_flag)"},
		{"(selected AND x)", "(selected AND x)"},
		{"(name = ')(')", "(name = ')(')"},
		{"(name = ')') AND (b = '(')", "((name = ')') AND (b = '('))"},
	}
	for _, tt := range tests {
		if got := ensureParentheses(tt.expr); got != tt.want {
			t.Errorf("ensureParentheses(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}
