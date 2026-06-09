package ir

import (
	"testing"
)

func TestIgnoreConfig_ShouldIgnoreTable(t *testing.T) {
	tests := []struct {
		name      string
		patterns  []string
		tableName string
		expected  bool
	}{
		{
			name:      "empty patterns",
			patterns:  []string{},
			tableName: "users",
			expected:  false,
		},
		{
			name:      "exact match",
			patterns:  []string{"temp_table"},
			tableName: "temp_table",
			expected:  true,
		},
		{
			name:      "no match",
			patterns:  []string{"temp_table"},
			tableName: "users",
			expected:  false,
		},
		{
			name:      "wildcard match - prefix",
			patterns:  []string{"temp_*"},
			tableName: "temp_users",
			expected:  true,
		},
		{
			name:      "wildcard match - suffix",
			patterns:  []string{"*_temp"},
			tableName: "users_temp",
			expected:  true,
		},
		{
			name:      "wildcard match - middle",
			patterns:  []string{"test_*_data"},
			tableName: "test_user_data",
			expected:  true,
		},
		{
			name:      "multiple patterns - first matches",
			patterns:  []string{"temp_*", "backup_*"},
			tableName: "temp_users",
			expected:  true,
		},
		{
			name:      "multiple patterns - second matches",
			patterns:  []string{"temp_*", "backup_*"},
			tableName: "backup_data",
			expected:  true,
		},
		{
			name:      "negation pattern - overrides inclusion",
			patterns:  []string{"test_*", "!test_core_users"},
			tableName: "test_core_users",
			expected:  false,
		},
		{
			name:      "negation pattern - inclusion still works",
			patterns:  []string{"test_*", "!test_core_users"},
			tableName: "test_temp_users",
			expected:  true,
		},
		{
			name:      "multiple negations",
			patterns:  []string{"temp_*", "!temp_core", "!temp_main"},
			tableName: "temp_core",
			expected:  false,
		},
		{
			name:      "multiple negations - other matches",
			patterns:  []string{"temp_*", "!temp_core", "!temp_main"},
			tableName: "temp_backup",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &IgnoreConfig{
				Tables: tt.patterns,
			}
			result := config.ShouldIgnoreTable(tt.tableName)
			if result != tt.expected {
				t.Errorf("ShouldIgnoreTable(%q) with patterns %v = %v, want %v",
					tt.tableName, tt.patterns, result, tt.expected)
			}
		})
	}
}

func TestIgnoreConfig_AllObjectTypes(t *testing.T) {
	config := &IgnoreConfig{
		Tables:      []string{"table_*"},
		Views:       []string{"view_*"},
		Functions:   []string{"fn_*"},
		Procedures:  []string{"sp_*"},
		Aggregates:  []string{"agg_*"},
		Types:       []string{"type_*"},
		Sequences:   []string{"seq_*"},
		Indexes:     []string{"idx_*"},
		Constraints: []string{"fk_*"},
		Triggers:    []string{"trg_*"},
	}

	// Test each object type
	tests := []struct {
		method   func(string) bool
		name     string
		expected bool
	}{
		{config.ShouldIgnoreTable, "table_temp", true},
		{config.ShouldIgnoreTable, "users", false},
		{config.ShouldIgnoreView, "view_temp", true},
		{config.ShouldIgnoreView, "user_view", false},
		{config.ShouldIgnoreFunction, "fn_temp", true},
		{config.ShouldIgnoreFunction, "get_user", false},
		{config.ShouldIgnoreProcedure, "sp_temp", true},
		{config.ShouldIgnoreProcedure, "process_data", false},
		{config.ShouldIgnoreAggregate, "agg_temp", true},
		{config.ShouldIgnoreAggregate, "group_concat", false},
		{config.ShouldIgnoreType, "type_temp", true},
		{config.ShouldIgnoreType, "user_status", false},
		{config.ShouldIgnoreSequence, "seq_temp", true},
		{config.ShouldIgnoreSequence, "user_id_seq", false},
		{config.ShouldIgnoreIndex, "idx_temp", true},
		{config.ShouldIgnoreIndex, "users_pkey", false},
		{config.ShouldIgnoreConstraint, "fk_orders_product", true},
		{config.ShouldIgnoreConstraint, "users_pkey", false},
		{config.ShouldIgnoreTrigger, "trg_audit", true},
		{config.ShouldIgnoreTrigger, "set_updated_at", false},
	}

	for _, tt := range tests {
		result := tt.method(tt.name)
		if result != tt.expected {
			t.Errorf("Method returned %v for %q, want %v", result, tt.name, tt.expected)
		}
	}
}

func TestIgnoreConfig_NilConfig(t *testing.T) {
	var config *IgnoreConfig = nil

	// All methods should return false for nil config
	if config.ShouldIgnoreTable("any_table") {
		t.Error("nil config should not ignore any table")
	}
	if config.ShouldIgnoreView("any_view") {
		t.Error("nil config should not ignore any view")
	}
	if config.ShouldIgnoreFunction("any_function") {
		t.Error("nil config should not ignore any function")
	}
	if config.ShouldIgnoreProcedure("any_procedure") {
		t.Error("nil config should not ignore any procedure")
	}
	if config.ShouldIgnoreAggregate("any_aggregate") {
		t.Error("nil config should not ignore any aggregate")
	}
	if config.ShouldIgnoreType("any_type") {
		t.Error("nil config should not ignore any type")
	}
	if config.ShouldIgnoreSequence("any_sequence") {
		t.Error("nil config should not ignore any sequence")
	}
	if config.ShouldIgnoreIndex("any_index") {
		t.Error("nil config should not ignore any index")
	}
	if config.ShouldIgnoreConstraint("any_constraint") {
		t.Error("nil config should not ignore any constraint")
	}
	if config.ShouldIgnoreTrigger("any_trigger") {
		t.Error("nil config should not ignore any trigger")
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		name     string
		expected bool
	}{
		{"exact", "exact", true},
		{"exact", "different", false},
		{"*", "anything", true},
		{"*", "", true},
		{"prefix_*", "prefix_suffix", true},
		{"prefix_*", "prefix_", true},
		{"prefix_*", "wrongprefix_suffix", false},
		{"*_suffix", "prefix_suffix", true},
		{"*_suffix", "_suffix", true},
		{"*_suffix", "prefix_wrong", false},
		{"a*c", "abc", true},
		{"a*c", "aXXXc", true},
		{"a*c", "ac", true},
		{"a*c", "ab", false},
		{"test_*_data", "test_user_data", true},
		{"test_*_data", "test_data", false},
		{"[invalid", "[invalid", true}, // Invalid pattern should fallback to literal match
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_vs_"+tt.name, func(t *testing.T) {
			result := matchPattern(tt.pattern, tt.name)
			if result != tt.expected {
				t.Errorf("matchPattern(%q, %q) = %v, want %v",
					tt.pattern, tt.name, result, tt.expected)
			}
		})
	}
}
