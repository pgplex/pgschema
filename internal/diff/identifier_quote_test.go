package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestGenerateConstraintSQL_WithQuoting(t *testing.T) {
	tests := []struct {
		name       string
		constraint *ir.Constraint
		want       string
	}{
		{
			name: "UNIQUE with camelCase columns",
			constraint: &ir.Constraint{
				Name: "test_unique",
				Type: ir.ConstraintTypeUnique,
				Columns: []*ir.ConstraintColumn{
					{Name: "userId", Position: 1},
					{Name: "accountId", Position: 2},
				},
			},
			want: `CONSTRAINT test_unique UNIQUE ("userId", "accountId")`,
		},
		{
			name: "PRIMARY KEY with reserved word",
			constraint: &ir.Constraint{
				Name: "test_pk",
				Type: ir.ConstraintTypePrimaryKey,
				Columns: []*ir.ConstraintColumn{
					{Name: "user", Position: 1},
					{Name: "order", Position: 2},
				},
			},
			want: `CONSTRAINT test_pk PRIMARY KEY ("user", "order")`,
		},
		{
			name: "FOREIGN KEY with camelCase",
			constraint: &ir.Constraint{
				Name: "test_fk",
				Type: ir.ConstraintTypeForeignKey,
				Columns: []*ir.ConstraintColumn{
					{Name: "userId", Position: 1},
				},
				ReferencedSchema: "public",
				ReferencedTable:  "users",
				ReferencedColumns: []*ir.ConstraintColumn{
					{Name: "id", Position: 1},
				},
				DeleteRule: "CASCADE",
				IsValid:    true,
			},
			want: `CONSTRAINT test_fk FOREIGN KEY ("userId") REFERENCES users (id) ON DELETE CASCADE`,
		},
		{
			name: "UNIQUE with lowercase columns (no quotes needed)",
			constraint: &ir.Constraint{
				Name: "test_unique_lower",
				Type: ir.ConstraintTypeUnique,
				Columns: []*ir.ConstraintColumn{
					{Name: "email", Position: 1},
					{Name: "username", Position: 2},
				},
			},
			want: `CONSTRAINT test_unique_lower UNIQUE (email, username)`,
		},
		{
			name: "FOREIGN KEY with cross-schema reference",
			constraint: &ir.Constraint{
				Name: "test_cross_schema_fk",
				Type: ir.ConstraintTypeForeignKey,
				Columns: []*ir.ConstraintColumn{
					{Name: "category_id", Position: 1},
				},
				ReferencedSchema: "public",
				ReferencedTable:  "categories",
				ReferencedColumns: []*ir.ConstraintColumn{
					{Name: "id", Position: 1},
				},
				IsValid: true,
			},
			want: `CONSTRAINT test_cross_schema_fk FOREIGN KEY (category_id) REFERENCES public.categories (id)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use "tenant1" as target schema for cross-schema FK test, "public" for others
			targetSchema := "public"
			if tt.name == "FOREIGN KEY with cross-schema reference" {
				targetSchema = "tenant1"
			}
			got := generateConstraintSQL(tt.constraint, targetSchema)
			if got != tt.want {
				t.Errorf("generateConstraintSQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckConstraintQuoting(t *testing.T) {
	tests := []struct {
		name       string
		constraint *ir.Constraint
		want       string
	}{
		{
			name: "CHECK with camelCase column",
			constraint: &ir.Constraint{
				Name:        "positive_followers",
				Type:        ir.ConstraintTypeCheck,
				CheckClause: `CHECK ("followerCount" >= 0)`,
				IsValid:     true,
			},
			want: `CONSTRAINT positive_followers CHECK ("followerCount" >= 0)`,
		},
		{
			name: "CHECK with multiple camelCase columns and AND",
			constraint: &ir.Constraint{
				Name:        "valid_counts",
				Type:        ir.ConstraintTypeCheck,
				CheckClause: `CHECK ("likeCount" >= 0 AND "commentCount" >= 0)`,
				IsValid:     true,
			},
			want: `CONSTRAINT valid_counts CHECK ("likeCount" >= 0 AND "commentCount" >= 0)`,
		},
		{
			name: "CHECK with BETWEEN",
			constraint: &ir.Constraint{
				Name:        "stock_range",
				Type:        ir.ConstraintTypeCheck,
				CheckClause: `CHECK ("stockLevel" BETWEEN 0 AND 1000)`,
				IsValid:     true,
			},
			want: `CONSTRAINT stock_range CHECK ("stockLevel" BETWEEN 0 AND 1000)`,
		},
		{
			name: "CHECK with IN clause",
			constraint: &ir.Constraint{
				Name:        "valid_status",
				Type:        ir.ConstraintTypeCheck,
				CheckClause: `CHECK ("orderStatus" IN ('pending', 'shipped', 'delivered'))`,
				IsValid:     true,
			},
			want: `CONSTRAINT valid_status CHECK ("orderStatus" IN ('pending', 'shipped', 'delivered'))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For CHECK constraints, generateConstraintSQL returns the CheckClause as-is
			got := generateConstraintSQL(tt.constraint, "public")
			if got != tt.want {
				t.Errorf("generateConstraintSQL() for CHECK = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateForeignKeyClause_WithQuoting(t *testing.T) {
	tests := []struct {
		name       string
		constraint *ir.Constraint
		want       string
	}{
		{
			name: "single referenced column camelCase",
			constraint: &ir.Constraint{
				Name:             "fk_single",
				Type:             ir.ConstraintTypeForeignKey,
				ReferencedSchema: "public",
				ReferencedTable:  "users",
				ReferencedColumns: []*ir.ConstraintColumn{
					{Name: "userId", Position: 1},
				},
			},
			want: `REFERENCES users ("userId")`,
		},
		{
			name: "multi referenced columns mixed-case and reserved word",
			constraint: &ir.Constraint{
				Name:             "fk_multi",
				Type:             ir.ConstraintTypeForeignKey,
				ReferencedSchema: "public",
				ReferencedTable:  "accounts",
				ReferencedColumns: []*ir.ConstraintColumn{
					{Name: "tenantId", Position: 1},
					{Name: "user", Position: 2},
				},
			},
			want: `REFERENCES accounts ("tenantId", "user")`,
		},
		{
			name: "single referenced column lowercase (no quotes)",
			constraint: &ir.Constraint{
				Name:             "fk_lower",
				Type:             ir.ConstraintTypeForeignKey,
				ReferencedSchema: "public",
				ReferencedTable:  "users",
				ReferencedColumns: []*ir.ConstraintColumn{
					{Name: "id", Position: 1},
				},
			},
			want: `REFERENCES users (id)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateForeignKeyClause(tt.constraint, "public", false)
			if got != tt.want {
				t.Errorf("generateForeignKeyClause() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateTriggerSQLWithMode_WithQuoting(t *testing.T) {
	tests := []struct {
		name    string
		trigger *ir.Trigger
		want    string
	}{
		{
			name: "mixed-case trigger name",
			trigger: &ir.Trigger{
				Schema:   "public",
				Table:    "orders",
				Name:     "trgAudit",
				Timing:   ir.TriggerTimingAfter,
				Events:   []ir.TriggerEvent{ir.TriggerEventInsert},
				Level:    ir.TriggerLevelRow,
				Function: "audit_fn()",
			},
			want: "CREATE OR REPLACE TRIGGER \"trgAudit\"\n" +
				"    AFTER INSERT ON orders\n" +
				"    FOR EACH ROW\n" +
				"    EXECUTE FUNCTION audit_fn();",
		},
		{
			name: "UPDATE OF mixed-case columns",
			trigger: &ir.Trigger{
				Schema:        "public",
				Table:         "orders",
				Name:          "track_changes",
				Timing:        ir.TriggerTimingAfter,
				Events:        []ir.TriggerEvent{ir.TriggerEventUpdate},
				UpdateColumns: []string{"userId", "email"},
				Level:         ir.TriggerLevelRow,
				Function:      "track_fn()",
			},
			want: "CREATE OR REPLACE TRIGGER track_changes\n" +
				"    AFTER UPDATE OF \"userId\", email ON orders\n" +
				"    FOR EACH ROW\n" +
				"    EXECUTE FUNCTION track_fn();",
		},
		{
			name: "mixed-case transition table aliases",
			trigger: &ir.Trigger{
				Schema:   "public",
				Table:    "orders",
				Name:     "audit_stmt",
				Timing:   ir.TriggerTimingAfter,
				Events:   []ir.TriggerEvent{ir.TriggerEventInsert},
				Level:    ir.TriggerLevelStatement,
				OldTable: "OldRows",
				NewTable: "NewRows",
				Function: "audit_stmt_fn()",
			},
			want: "CREATE OR REPLACE TRIGGER audit_stmt\n" +
				"    AFTER INSERT ON orders\n" +
				"    REFERENCING OLD TABLE AS \"OldRows\" NEW TABLE AS \"NewRows\"\n" +
				"    FOR EACH STATEMENT\n" +
				"    EXECUTE FUNCTION audit_stmt_fn();",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTriggerSQLWithMode(tt.trigger, "public")
			if got != tt.want {
				t.Errorf("generateTriggerSQLWithMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddColumnIdentifierQuoting(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
		wantQuoted bool
	}{
		{"camelCase column", "followerCount", true},
		{"PascalCase column", "IsVerified", true},
		{"lowercase column", "follower_count", false},
		{"reserved word", "user", true},
		{"with numbers", "column123", false},
		{"starts with uppercase", "Column", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quoted := ir.QuoteIdentifier(tt.columnName)
			hasQuotes := quoted[0] == '"' && quoted[len(quoted)-1] == '"'

			if hasQuotes != tt.wantQuoted {
				t.Errorf("ir.QuoteIdentifier(%q) = %q, want quoted: %v",
					tt.columnName, quoted, tt.wantQuoted)
			}
		})
	}
}
