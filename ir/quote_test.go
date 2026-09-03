package ir

import (
	"fmt"
	"testing"
)

func TestQuoteIdentifier(t *testing.T) {
	type testCase struct {
		name       string
		identifier string
		expected   string
	}

	tests := []testCase{
		{"simple lowercase", "users", "users"},
		{"camelCase", "firstName", `"firstName"`},
		{"UPPERCASE", "USERS", `"USERS"`},
		{"MixedCase", "MyApp", `"MyApp"`},
		{"with underscore", "user_name", "user_name"},
		{"starts with underscore", "_private", "_private"},
		{"starts with number", "1table", `"1table"`},
		{"contains dash", "user-table", `"user-table"`},
		{"embedded double quote", `My"Func`, `"My""Func"`},
		{"empty string", "", ""},
	}

	// adding all keywords as test cases to ensure all values are checked, there may be some duplicates
	// in the tests above to ensure that there is also the opportunity to visually inspect a subset
	// of the test cases.
	for reservedWord := range reservedWords {
		tests = append(tests, testCase{
			name:       fmt.Sprintf("reserved word: %q", reservedWord),
			identifier: reservedWord,
			expected:   fmt.Sprintf(`"%s"`, reservedWord),
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QuoteIdentifier(tt.identifier)
			if result != tt.expected {
				t.Errorf("QuoteIdentifier(%q) = %q; want %q", tt.identifier, result, tt.expected)
			}
		})
	}
}

func TestQualifyEntityNameWithQuotes(t *testing.T) {
	tests := []struct {
		name         string
		entitySchema string
		entityName   string
		targetSchema string
		expected     string
	}{
		{"same schema lowercase", "public", "users", "public", "users"},
		{"same schema mixed case", "MyApp", "Orders", "MyApp", `"Orders"`},
		{"different schema lowercase", "tenant", "users", "public", "tenant.users"},
		{"different schema mixed case", "MyApp", "Orders", "public", `"MyApp"."Orders"`},
		{"reserved word schema", "user", "table", "public", `"user"."table"`},
		{"mixed case schema with lowercase target", "MyApp", "users", "public", `"MyApp".users`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QualifyEntityNameWithQuotes(tt.entitySchema, tt.entityName, tt.targetSchema)
			if result != tt.expected {
				t.Errorf("QualifyEntityNameWithQuotes(%q, %q, %q) = %q; want %q", tt.entitySchema, tt.entityName, tt.targetSchema, result, tt.expected)
			}
		})
	}
}
