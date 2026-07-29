package diff

import (
	"strings"
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

// A sequence that is referenced by a column's nextval() default (populating
// OwnedByTable/OwnedByColumn via the name-matching fallback) but NOT genuinely
// owned (IsOwned=false) must still be compared structurally - a real
// increment/min/max/cycle change on such a sequence must not be silently
// dropped just because it looks "owned" by the loose OwnedByTable/Column
// check.
// A sequence genuinely owned (IsOwned=true) by a column of an ignore-filtered
// table must not be classified as a genuine ADD/DROP: ignore-filtered tables
// are deliberately absent from oldTables/newTables (see inspector.go's
// ShouldIgnoreTable), even though their sequence is still present in the
// Sequences map (sequences are filtered by their own name pattern, not their
// owning table's). Relying on columnHasOwnedSequence(tables, ...) - which
// requires the owning table to be present in that map - previously
// misclassified such a sequence as needing an explicit DROP SEQUENCE, even
// though nothing about the ignored table actually changed.
func TestGenerateMigration_IgnoredTableOwnedSequenceNotDropped(t *testing.T) {
	oldIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name:      "public",
				Tables:    map[string]*ir.Table{}, // temp_backup itself is ignore-filtered
				Views:     map[string]*ir.View{},
				Functions: map[string]*ir.Function{},
				Sequences: map[string]*ir.Sequence{
					"temp_backup_id_seq": {
						Schema: "public", Name: "temp_backup_id_seq", DataType: "integer",
						StartValue: 1, Increment: 1,
						OwnedByTable: "temp_backup", OwnedByColumn: "id", IsOwned: true,
					},
				},
				Types: map[string]*ir.Type{},
			},
		},
	}
	newIR := &ir.IR{
		Schemas: map[string]*ir.Schema{
			"public": {
				Name:      "public",
				Tables:    map[string]*ir.Table{},
				Views:     map[string]*ir.View{},
				Functions: map[string]*ir.Function{},
				Sequences: map[string]*ir.Sequence{}, // never introspected - the ignored table was never created in the plan-time scratch db either
				Types:     map[string]*ir.Type{},
			},
		},
	}

	diffs := GenerateMigration(oldIR, newIR, "public")

	for _, d := range diffs {
		for _, stmt := range d.Statements {
			if strings.Contains(stmt.SQL, "DROP SEQUENCE") && strings.Contains(stmt.SQL, "temp_backup_id_seq") {
				t.Errorf("expected no explicit DROP SEQUENCE for an ignore-filtered table's genuinely-owned sequence, got: %q", stmt.SQL)
			}
		}
	}
}

func TestGenerateMigration_ModifiedNonOwnedSequenceStructuralChangeDetected(t *testing.T) {
	buildIR := func(increment int64) *ir.IR {
		return &ir.IR{
			Schemas: map[string]*ir.Schema{
				"public": {
					Name:      "public",
					Tables:    map[string]*ir.Table{},
					Views:     map[string]*ir.View{},
					Functions: map[string]*ir.Function{},
					Sequences: map[string]*ir.Sequence{
						"shared_seq": {
							Schema: "public", Name: "shared_seq", DataType: "bigint",
							StartValue: 1, Increment: increment,
							OwnedByTable: "sharer", OwnedByColumn: "owner_id", IsOwned: false,
						},
					},
					Types: map[string]*ir.Type{},
				},
			},
		}
	}

	oldIR := buildIR(1)
	newIR := buildIR(2)

	diffs := GenerateMigration(oldIR, newIR, "public")

	found := false
	for _, d := range diffs {
		for _, stmt := range d.Statements {
			if strings.Contains(stmt.SQL, "ALTER SEQUENCE") && strings.Contains(stmt.SQL, "INCREMENT") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected an ALTER SEQUENCE ... INCREMENT statement for a structurally-changed, non-owned sequence, got diffs: %+v", diffs)
	}
}

