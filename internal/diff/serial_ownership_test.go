package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

// A column can have a nextval() default without genuinely owning that sequence
// (e.g. two tables sharing one sequence, or a standalone sequence never tied to
// any column via OWNED BY). isSerialColumn - and everything downstream that
// depends on it (SERIAL/BIGSERIAL/SMALLSERIAL rendering, skipping an explicit
// CREATE SEQUENCE, and OWNED BY emission) - must only fire for genuine
// ownership, verified via a real pg_depend edge (ir.Column.HasOwnedSequence /
// ir.Sequence.IsOwned), not merely because the default text mentions nextval().

func TestIsSerialColumn(t *testing.T) {
	nextvalDefault := `nextval('"foo_id_seq"'::regclass)`
	plainDefault := "0"

	tests := []struct {
		name   string
		column *ir.Column
		want   bool
	}{
		{
			name:   "integer with owned sequence -> serial",
			column: &ir.Column{DataType: "integer", DefaultValue: &nextvalDefault, HasOwnedSequence: true},
			want:   true,
		},
		{
			name:   "bigint with owned sequence -> serial",
			column: &ir.Column{DataType: "bigint", DefaultValue: &nextvalDefault, HasOwnedSequence: true},
			want:   true,
		},
		{
			name:   "integer with nextval default but NOT owned -> not serial",
			column: &ir.Column{DataType: "integer", DefaultValue: &nextvalDefault, HasOwnedSequence: false},
			want:   false,
		},
		{
			name:   "integer with no default -> not serial",
			column: &ir.Column{DataType: "integer", HasOwnedSequence: true},
			want:   false,
		},
		{
			name:   "integer with non-nextval default -> not serial",
			column: &ir.Column{DataType: "integer", DefaultValue: &plainDefault, HasOwnedSequence: true},
			want:   false,
		},
		{
			name:   "non-integer type, owned sequence -> not serial",
			column: &ir.Column{DataType: "text", DefaultValue: &nextvalDefault, HasOwnedSequence: true},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSerialColumn(tt.column)
			if got != tt.want {
				t.Errorf("isSerialColumn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateSequenceSQL_OwnedByRequiresIsOwned(t *testing.T) {
	tests := []struct {
		name string
		seq  *ir.Sequence
		want string
	}{
		{
			name: "OwnedByTable/Column set but IsOwned false -> no OWNED BY clause",
			seq: &ir.Sequence{
				Schema: "public", Name: "shared_seq", StartValue: 1, Increment: 1,
				OwnedByTable: "other_table", OwnedByColumn: "other_col", IsOwned: false,
			},
			want: "CREATE SEQUENCE IF NOT EXISTS shared_seq;",
		},
		{
			name: "OwnedByTable/Column set and IsOwned true -> OWNED BY clause emitted",
			seq: &ir.Sequence{
				Schema: "public", Name: "orders_id_seq", StartValue: 1, Increment: 1,
				OwnedByTable: "orders", OwnedByColumn: "id", IsOwned: true,
			},
			want: "CREATE SEQUENCE IF NOT EXISTS orders_id_seq OWNED BY orders.id;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSequenceSQL(tt.seq, "public", false)
			if got != tt.want {
				t.Errorf("generateSequenceSQL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestColumnHasOwnedSequence(t *testing.T) {
	tables := map[string]*ir.Table{
		"public.orders": {
			Schema: "public", Name: "orders",
			Columns: []*ir.Column{
				{Name: "id", HasOwnedSequence: true},
				{Name: "shared_id", HasOwnedSequence: false},
			},
		},
	}

	if !columnHasOwnedSequence(tables, "public", "orders", "id") {
		t.Errorf("columnHasOwnedSequence() = false for genuinely owned column, want true")
	}
	if columnHasOwnedSequence(tables, "public", "orders", "shared_id") {
		t.Errorf("columnHasOwnedSequence() = true for non-owning column, want false")
	}
	if columnHasOwnedSequence(tables, "public", "orders", "does_not_exist") {
		t.Errorf("columnHasOwnedSequence() = true for missing column, want false")
	}
	if columnHasOwnedSequence(tables, "public", "missing_table", "id") {
		t.Errorf("columnHasOwnedSequence() = true for missing table, want false")
	}
}
