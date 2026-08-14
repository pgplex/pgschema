package diff

import "testing"

func TestNormalizeBaseTypeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"integer", "integer"},
		{"INTEGER", "integer"},
		{"bigint", "bigint"},
		{"numeric(18,6)", "numeric"},
		{"numeric(20,6)", "numeric"},
		{"varchar(128)", "varchar"},
		{"character varying(255)", "character varying"},
		{"integer[]", "integer[]"},
		{"character varying(128)[]", "character varying[]"},
		{"timestamp(6) with time zone", "timestamp with time zone"},
		{"pg_catalog.int4", "int4"},
		{`"MyStatus"`, `"MyStatus"`},
		{`"mystatus"`, `"mystatus"`},
		{`"MyStatus"[]`, `"MyStatus"[]`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeBaseTypeName(tt.input)
			if got != tt.want {
				t.Errorf("normalizeBaseTypeName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNeedsUsingClause(t *testing.T) {
	tests := []struct {
		old  string
		new  string
		want bool
	}{
		{"text", "integer", true},
		{"integer", "bigint", true},
		{"numeric(18,6)", "numeric(20,6)", false},
		{"integer", "integer[]", true},
		{"text", "action_type", true},
		{"varchar(128)", "varchar(255)", false},
		{"timestamp(3) with time zone", "timestamp(6) with time zone", false},
		{"timestamp without time zone", "timestamp with time zone", true},
		{`"MyStatus"`, `"mystatus"`, true},
	}
	for _, tt := range tests {
		t.Run(tt.old+"→"+tt.new, func(t *testing.T) {
			got := needsUsingClause(tt.old, tt.new)
			if got != tt.want {
				t.Errorf("needsUsingClause(%q, %q) = %v, want %v", tt.old, tt.new, got, tt.want)
			}
		})
	}
}
