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
