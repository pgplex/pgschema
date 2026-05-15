package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestColumnsMatchForRename(t *testing.T) {
	boolDefault := "false"

	tests := []struct {
		name     string
		old      *ir.Column
		new      *ir.Column
		schema   string
		expected bool
	}{
		{
			name:     "same type and position, different name = match",
			old:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			new:      &ir.Column{Name: "active_n", Position: 3, DataType: "boolean", IsNullable: false},
			expected: true,
		},
		{
			name:     "different type = no match",
			old:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			new:      &ir.Column{Name: "active_n", Position: 3, DataType: "integer", IsNullable: false},
			expected: false,
		},
		{
			name:     "different position = no match",
			old:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			new:      &ir.Column{Name: "active_n", Position: 4, DataType: "boolean", IsNullable: false},
			expected: false,
		},
		{
			name:     "different nullability = no match",
			old:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			new:      &ir.Column{Name: "active_n", Position: 3, DataType: "boolean", IsNullable: true},
			expected: false,
		},
		{
			name:     "same name = no match (not a rename)",
			old:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			new:      &ir.Column{Name: "active", Position: 3, DataType: "boolean", IsNullable: false},
			expected: false,
		},
		{
			name:     "with matching defaults = match",
			old:      &ir.Column{Name: "old_col", Position: 2, DataType: "boolean", DefaultValue: &boolDefault},
			new:      &ir.Column{Name: "new_col", Position: 2, DataType: "boolean", DefaultValue: &boolDefault},
			expected: true,
		},
		{
			name: "with matching identity = match",
			old:  &ir.Column{Name: "old_id", Position: 1, DataType: "integer", Identity: &ir.Identity{Generation: "ALWAYS"}},
			new:  &ir.Column{Name: "new_id", Position: 1, DataType: "integer", Identity: &ir.Identity{Generation: "ALWAYS"}},
			expected: true,
		},
		{
			name: "different identity generation = no match",
			old:  &ir.Column{Name: "old_id", Position: 1, DataType: "integer", Identity: &ir.Identity{Generation: "ALWAYS"}},
			new:  &ir.Column{Name: "new_id", Position: 1, DataType: "integer", Identity: &ir.Identity{Generation: "BY DEFAULT"}},
			expected: false,
		},
		{
			name:     "schema-qualified type normalized = match",
			old:      &ir.Column{Name: "old_col", Position: 1, DataType: "public.my_type"},
			new:      &ir.Column{Name: "new_col", Position: 1, DataType: "my_type"},
			schema:   "public",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := columnsMatchForRename(tt.old, tt.new, tt.schema)
			if result != tt.expected {
				t.Errorf("columnsMatchForRename() = %v, want %v", result, tt.expected)
			}
		})
	}
}
