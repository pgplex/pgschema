package diff

import "testing"

// TestReferencesNewFunctionQuotedIdentifiers covers issue #571: functionCallRegex
// previously only matched unquoted identifiers, so a case-sensitive, double-quoted
// function name (and one containing an escaped "" quote) was never recognized as
// a dependency.
func TestReferencesNewFunctionQuotedIdentifiers(t *testing.T) {
	tests := []struct {
		name          string
		expr          string
		defaultSchema string
		newFunctions  map[string]struct{}
		want          bool
	}{
		{
			name:          "unquoted call",
			expr:          "myfunc()",
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"public.myfunc": {}},
			want:          true,
		},
		{
			name:          "quoted case-sensitive call",
			expr:          `"MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"public.myfunc": {}},
			want:          true,
		},
		{
			name:          "quoted schema and quoted function",
			expr:          `"MySchema"."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"myschema.myfunc": {}},
			want:          true,
		},
		{
			name:          "unquoted schema, quoted function",
			expr:          `myschema."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"myschema.myfunc": {}},
			want:          true,
		},
		{
			name:          "quoted identifier with escaped quote",
			expr:          `"My""Func"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{`public.my"func`: {}},
			want:          true,
		},
		{
			name:          "quoted call to unrelated function",
			expr:          `"OtherFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"public.myfunc": {}},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := referencesNewFunction(tt.expr, tt.defaultSchema, tt.newFunctions)
			if got != tt.want {
				t.Errorf("referencesNewFunction(%q, %q) = %v, want %v", tt.expr, tt.defaultSchema, got, tt.want)
			}
		})
	}
}

func TestNormalizeFunctionIdentifier(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{`myfunc`, "myfunc"},
		{`"MyFunc"`, "myfunc"},
		{`"MySchema"."MyFunc"`, "myschema.myfunc"},
		{`myschema."MyFunc"`, "myschema.myfunc"},
		{`"My""Func"`, `my"func`},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := normalizeFunctionIdentifier(tt.raw)
			if got != tt.want {
				t.Errorf("normalizeFunctionIdentifier(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
