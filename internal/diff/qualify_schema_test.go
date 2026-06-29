package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// These tests cover the `dump --qualify-schema` behavior at the SQL-builder level:
// when qualifySchema is true, structural entity names/types are always schema-qualified,
// even for the target schema; when false (the default), the standard "smart
// qualification" omits the target-schema prefix. The false cases double as the
// byte-identical default-output guardrail for each builder.

func TestQualifySchema_ForeignKeyReference(t *testing.T) {
	fk := &ir.Constraint{
		Name: "fk_user",
		Type: ir.ConstraintTypeForeignKey,
		Columns: []*ir.ConstraintColumn{
			{Name: "user_id", Position: 1},
		},
		ReferencedSchema: "public",
		ReferencedTable:  "users",
		ReferencedColumns: []*ir.ConstraintColumn{
			{Name: "id", Position: 1},
		},
		IsValid: true,
	}

	// Default: the target-schema prefix is omitted on the referenced table.
	if got, want := generateConstraintSQL(fk, "public", false),
		`CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users (id)`; got != want {
		t.Errorf("default: got %q, want %q", got, want)
	}
	// Forced qualification: the referenced table keeps its schema prefix.
	if got, want := generateConstraintSQL(fk, "public", true),
		`CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES public.users (id)`; got != want {
		t.Errorf("qualified: got %q, want %q", got, want)
	}
}

func TestQualifySchema_TableAndColumnType(t *testing.T) {
	table := &ir.Table{
		Schema: "public",
		Name:   "account",
		Columns: []*ir.Column{
			{Name: "id", Position: 1, DataType: "integer", IsNullable: false},
			// A column whose type lives in the target schema (e.g. a user-defined enum).
			{Name: "kind", Position: 2, DataType: "public.user_kind", IsNullable: true},
		},
	}
	empty := map[string]bool{}

	def, _ := generateTableSQL(table, "public", false, empty, empty, empty)
	if !strings.Contains(def, "CREATE TABLE IF NOT EXISTS account (") {
		t.Errorf("default should use the bare table name: %q", def)
	}
	if strings.Contains(def, "public.account") || strings.Contains(def, "public.user_kind") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}

	qualified, _ := generateTableSQL(table, "public", true, empty, empty, empty)
	if !strings.Contains(qualified, "CREATE TABLE IF NOT EXISTS public.account (") {
		t.Errorf("forced qualification should qualify the table name: %q", qualified)
	}
	if !strings.Contains(qualified, "public.user_kind") {
		t.Errorf("forced qualification should qualify the column type: %q", qualified)
	}
}

func TestQualifySchema_Type(t *testing.T) {
	typ := &ir.Type{
		Schema:     "public",
		Name:       "status",
		Kind:       ir.TypeKindEnum,
		EnumValues: []string{"active", "inactive"},
	}

	def := generateTypeSQL(typ, "public", false)
	if !strings.Contains(def, "CREATE TYPE status AS ENUM") {
		t.Errorf("default should use the bare type name: %q", def)
	}
	if strings.Contains(def, "public.status") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}

	qualified := generateTypeSQL(typ, "public", true)
	if !strings.Contains(qualified, "CREATE TYPE public.status AS ENUM") {
		t.Errorf("forced qualification should qualify the type name: %q", qualified)
	}
}

func TestQualifySchema_Sequence(t *testing.T) {
	seq := &ir.Sequence{
		Schema:        "public",
		Name:          "users_id_seq",
		StartValue:    1,
		Increment:     1,
		OwnedByTable:  "users",
		OwnedByColumn: "id",
	}

	def := generateSequenceSQL(seq, "public", false)
	if strings.Contains(def, "public.users_id_seq") || strings.Contains(def, "public.users") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}

	qualified := generateSequenceSQL(seq, "public", true)
	if !strings.Contains(qualified, "public.users_id_seq") {
		t.Errorf("forced qualification should qualify the sequence name: %q", qualified)
	}
	if !strings.Contains(qualified, "OWNED BY public.users") {
		t.Errorf("forced qualification should qualify the OWNED BY table: %q", qualified)
	}
}

func TestQualifySchema_IndexOnTable(t *testing.T) {
	idx := &ir.Index{
		Schema:  "public",
		Table:   "account",
		Name:    "idx_account_kind",
		Type:    ir.IndexTypeRegular,
		Method:  "btree",
		Columns: []*ir.IndexColumn{{Name: "kind", Position: 1}},
	}

	// Default: the CREATE INDEX ... ON reference uses the bare table name.
	def := generateIndexSQLMode(idx, "public", false, false)
	if !strings.Contains(def, " ON account ") {
		t.Errorf("default should use the bare ON-table name: %q", def)
	}
	if strings.Contains(def, "public.account") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}

	// Forced qualification: the ON-table reference keeps its schema prefix.
	qualified := generateIndexSQLMode(idx, "public", false, true)
	if !strings.Contains(qualified, " ON public.account ") {
		t.Errorf("forced qualification should qualify the ON-table name: %q", qualified)
	}
}

func TestQualifySchema_PolicyOnTable(t *testing.T) {
	policy := &ir.RLSPolicy{
		Schema:     "public",
		Table:      "account",
		Name:       "account_isolation",
		Command:    ir.PolicyCommandAll,
		Permissive: true,
		Using:      "(owner = current_user)",
	}

	// Default: the CREATE POLICY ... ON reference uses the bare table name.
	def := generatePolicySQLMode(policy, "public", false)
	if !strings.Contains(def, " ON account ") {
		t.Errorf("default should use the bare ON-table name: %q", def)
	}
	if strings.Contains(def, "public.account") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}

	// Forced qualification: the ON-table reference keeps its schema prefix.
	qualified := generatePolicySQLMode(policy, "public", true)
	if !strings.Contains(qualified, " ON public.account ") {
		t.Errorf("forced qualification should qualify the ON-table name: %q", qualified)
	}
}

func TestQualifySchema_Procedure(t *testing.T) {
	defaultVal := "0"
	proc := &ir.Procedure{
		Schema:   "public",
		Name:     "adjust_balance",
		Language: "plpgsql",
		// A parameter whose type lives in the target schema (e.g. a domain).
		Parameters: []*ir.Parameter{
			{Name: "amount", DataType: "public.currency", Mode: "IN", Position: 1, DefaultValue: &defaultVal},
		},
		Definition: "BEGIN END;",
	}

	def := generateProcedureSQL(proc, "public", false)
	if !strings.Contains(def, "CREATE OR REPLACE PROCEDURE adjust_balance") {
		t.Errorf("default should use the bare procedure name: %q", def)
	}
	if strings.Contains(def, "public.adjust_balance") || strings.Contains(def, "public.currency") {
		t.Errorf("default should not qualify the target schema: %q", def)
	}
	if !strings.Contains(def, "amount currency") {
		t.Errorf("default should strip the target-schema prefix from the param type: %q", def)
	}

	qualified := generateProcedureSQL(proc, "public", true)
	if !strings.Contains(qualified, "CREATE OR REPLACE PROCEDURE public.adjust_balance") {
		t.Errorf("forced qualification should qualify the procedure name: %q", qualified)
	}
	if !strings.Contains(qualified, "amount public.currency") {
		t.Errorf("forced qualification should qualify the param type: %q", qualified)
	}
	if !strings.Contains(qualified, "DEFAULT 0") {
		t.Errorf("forced qualification should preserve the DEFAULT clause: %q", qualified)
	}
}

func TestQualifySchema_DeferredForeignKeyClause(t *testing.T) {
	fk := &ir.Constraint{
		Name:             "fk_account_owner",
		Type:             ir.ConstraintTypeForeignKey,
		ReferencedSchema: "public",
		ReferencedTable:  "users",
		ReferencedColumns: []*ir.ConstraintColumn{
			{Name: "id", Position: 1},
		},
		IsValid: true,
	}

	// Default: the deferred FK REFERENCES uses the bare referenced-table name.
	if got := generateForeignKeyClauseMode(fk, "public", false, false); !strings.Contains(got, "REFERENCES users (id)") {
		t.Errorf("default should use the bare referenced table: %q", got)
	}
	// Forced qualification: the referenced table keeps its schema prefix.
	if got := generateForeignKeyClauseMode(fk, "public", false, true); !strings.Contains(got, "REFERENCES public.users (id)") {
		t.Errorf("forced qualification should qualify the referenced table: %q", got)
	}
}
