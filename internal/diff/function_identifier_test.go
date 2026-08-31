package diff

import "testing"

// TestReferencesNewFunctionQuotedIdentifiers covers issue #571: functionCallRegex
// previously only matched unquoted identifiers, so a case-sensitive, double-quoted
// function name (and one containing an escaped "" quote) was never recognized as
// a dependency. It also covers the follow-up review feedback that quoted and
// unquoted references must not be conflated: PostgreSQL only folds *unquoted*
// identifiers to lowercase, so "myfunc"() and "MyFunc"() are distinct functions.
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
			newFunctions:  map[string]struct{}{"public.MyFunc": {}},
			want:          true,
		},
		{
			name:          "quoted schema and quoted function",
			expr:          `"MySchema"."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"MySchema.MyFunc": {}},
			want:          true,
		},
		{
			name:          "unquoted schema, quoted function",
			expr:          `myschema."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"myschema.MyFunc": {}},
			want:          true,
		},
		{
			name:          "quoted identifier with escaped quote",
			expr:          `"My""Func"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{`public.My"Func`: {}},
			want:          true,
		},
		{
			name:          "quoted call to unrelated function",
			expr:          `"OtherFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"public.MyFunc": {}},
			want:          false,
		},
		{
			name:          "unquoted call does not match a quoted mixed-case function of the same letters",
			expr:          "myfunc()",
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{"public.MyFunc": {}},
			want:          false,
		},
		{
			name:          "quoted call does not match an unquoted lowercase function of the same letters",
			expr:          `"MyFunc"()`,
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
		{`"MyFunc"`, "MyFunc"},
		{`"MySchema"."MyFunc"`, "MySchema.MyFunc"},
		{`myschema."MyFunc"`, "myschema.MyFunc"},
		{`"My""Func"`, `My"Func`},
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

func TestFunctionLookupKeyPart(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"myfunc", "myfunc"},
		{"public", "public"},
		{"MyFunc", "MyFunc"},
		{"MySchema", "MySchema"},
		{`My"Func`, `My"Func`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := functionLookupKeyPart(tt.name)
			if got != tt.want {
				t.Errorf("functionLookupKeyPart(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
