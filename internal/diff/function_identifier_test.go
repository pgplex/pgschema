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
			newFunctions:  map[string]struct{}{functionGraphKey("public", "myfunc"): {}},
			want:          true,
		},
		{
			name:          "quoted case-sensitive call",
			expr:          `"MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "quoted schema and quoted function",
			expr:          `"MySchema"."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("MySchema", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "unquoted schema, quoted function",
			expr:          `myschema."MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("myschema", "MyFunc"): {}},
			want:          true,
		},
		{
			name:          "quoted identifier with escaped quote",
			expr:          `"My""Func"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", `My"Func`): {}},
			want:          true,
		},
		{
			name:          "quoted call to unrelated function",
			expr:          `"OtherFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          false,
		},
		{
			name:          "unquoted call does not match a quoted mixed-case function of the same letters",
			expr:          "myfunc()",
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "MyFunc"): {}},
			want:          false,
		},
		{
			name:          "quoted call does not match an unquoted lowercase function of the same letters",
			expr:          `"MyFunc"()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("public", "myfunc"): {}},
			want:          false,
		},
		{
			name:          "quoted schema containing a literal dot does not collide with a different qualified split",
			expr:          `"a.b".c()`,
			defaultSchema: "public",
			newFunctions:  map[string]struct{}{functionGraphKey("a", "b.c"): {}},
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
		{`"MySchema"."MyFunc"`, functionGraphKey("MySchema", "MyFunc")},
		{`myschema."MyFunc"`, functionGraphKey("myschema", "MyFunc")},
		{`"My""Func"`, `My"Func`},
		{`"a.b".c`, functionGraphKey("a.b", "c")},
		{`a."b.c"`, functionGraphKey("a", "b.c")},
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

func TestNormalizeFunctionIdentifierNoCollision(t *testing.T) {
	// A plain "."-joined key would flatten both of these to "a.b.c"; the
	// NUL-separated functionGraphKey must keep them distinct.
	quotedSchema := normalizeFunctionIdentifier(`"a.b".c`)
	quotedName := normalizeFunctionIdentifier(`a."b.c"`)
	if quotedSchema == quotedName {
		t.Errorf("normalizeFunctionIdentifier(%q) and (%q) collided: both produced %q", `"a.b".c`, `a."b.c"`, quotedSchema)
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
