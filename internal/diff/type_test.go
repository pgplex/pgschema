package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestDefaultMultirangeTypeName(t *testing.T) {
	cases := map[string]string{
		"timerange":  "timemultirange",
		"my_range_t": "my_multirange_t",
		"floatspan":  "floatspan_multirange",
		"rangerange": "multirangerange", // only the first occurrence
	}
	for in, want := range cases {
		if got := defaultMultirangeTypeName(in); got != want {
			t.Errorf("defaultMultirangeTypeName(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("a", 60) + "span"
	if got := defaultMultirangeTypeName(long); len(got) != 63 || !strings.HasPrefix(got, strings.Repeat("a", 60)) {
		t.Errorf("long name not clipped to 63 bytes: %q (%d)", got, len(got))
	}
}

func TestGenerateTypeSQLRange(t *testing.T) {
	cases := []struct {
		name string
		typ  *ir.Type
		want string
	}{
		{
			name: "default multirange name is omitted",
			typ: &ir.Type{
				Schema: "public", Name: "timerange", Kind: ir.TypeKindRange,
				Subtype: "time without time zone", MultirangeTypeName: "timemultirange",
				SubtypeDiff: "time_diff",
			},
			want: "CREATE TYPE timerange AS RANGE (\n    subtype = time without time zone,\n    subtype_diff = time_diff\n);",
		},
		{
			name: "explicit multirange name, opclass and collation",
			typ: &ir.Type{
				Schema: "public", Name: "textrange", Kind: ir.TypeKindRange,
				Subtype: "text", MultirangeTypeName: "textspans",
				SubtypeOpclass: "pg_catalog.text_pattern_ops", Collation: `"C"`,
			},
			want: "CREATE TYPE textrange AS RANGE (\n    subtype = text,\n    multirange_type_name = textspans,\n    subtype_opclass = pg_catalog.text_pattern_ops,\n    collation = \"C\"\n);",
		},
		{
			name: "multirange in another schema is printed qualified",
			typ: &ir.Type{
				Schema: "public", Name: "floatspan", Kind: ir.TypeKindRange,
				Subtype: "double precision", MultirangeTypeName: "other.floatspan_multirange",
			},
			want: "CREATE TYPE floatspan AS RANGE (\n    subtype = double precision,\n    multirange_type_name = other.floatspan_multirange\n);",
		},
		{
			name: "same-schema subtype prefix is stripped",
			typ: &ir.Type{
				Schema: "public", Name: "band", Kind: ir.TypeKindRange,
				Subtype: "public.severity", MultirangeTypeName: "band_multirange",
			},
			want: "CREATE TYPE band AS RANGE (\n    subtype = severity\n);",
		},
	}
	for _, tc := range cases {
		if got := generateTypeSQL(tc.typ, "public", false); got != tc.want {
			t.Errorf("%s:\n got: %q\nwant: %q", tc.name, got, tc.want)
		}
	}
}

func TestTypesEqualRange(t *testing.T) {
	base := func() *ir.Type {
		return &ir.Type{
			Schema: "public", Name: "r", Kind: ir.TypeKindRange,
			Subtype: "integer", MultirangeTypeName: "rmulti", SubtypeDiff: "d",
		}
	}
	if !typesEqual(base(), base()) {
		t.Fatal("identical ranges compare unequal")
	}
	for name, mutate := range map[string]func(*ir.Type){
		"subtype":    func(r *ir.Type) { r.Subtype = "bigint" },
		"opclass":    func(r *ir.Type) { r.SubtypeOpclass = "pg_catalog.int4_ops" },
		"collation":  func(r *ir.Type) { r.Collation = `"C"` },
		"canonical":  func(r *ir.Type) { r.Canonical = "c" },
		"diff":       func(r *ir.Type) { r.SubtypeDiff = "" },
		"multirange": func(r *ir.Type) { r.MultirangeTypeName = "other" },
	} {
		changed := base()
		mutate(changed)
		if typesEqual(base(), changed) {
			t.Errorf("%s change not detected", name)
		}
	}
}

func TestTopologicallySortTypesRange(t *testing.T) {
	types := []*ir.Type{
		// Dependents sort before the range by name, so only the edges order them.
		{Schema: "public", Name: "availability", Kind: ir.TypeKindDomain, BaseType: "public.period_multirange"},
		{Schema: "public", Name: "booking", Kind: ir.TypeKindComposite, Columns: []*ir.TypeColumn{{Name: "slot", DataType: "public.period"}}},
		{Schema: "public", Name: "period", Kind: ir.TypeKindRange, Subtype: "public.severity", MultirangeTypeName: "period_multirange"},
		{Schema: "public", Name: "severity", Kind: ir.TypeKindEnum, EnumValues: []string{"low", "high"}},
	}

	sorted := topologicallySortTypes(types)
	order := make(map[string]int, len(sorted))
	for idx, typ := range sorted {
		order[typ.Name] = idx
	}
	assertBefore := func(first, second string) {
		if order[first] >= order[second] {
			t.Fatalf("expected %s before %s in %v", first, second, order)
		}
	}
	assertBefore("severity", "period")
	assertBefore("period", "booking")
	assertBefore("period", "availability")
}

func TestHoldBackFunctionsOfDroppedTypes(t *testing.T) {
	diffFn := &ir.Function{Schema: "public", Name: "time_diff"}
	checkFn := &ir.Function{Schema: "public", Name: "is_valid"}
	other := &ir.Function{Schema: "public", Name: "unrelated"}
	types := []*ir.Type{
		{Schema: "public", Name: "timerange", Kind: ir.TypeKindRange, SubtypeDiff: "time_diff"},
		{Schema: "public", Name: "code", Kind: ir.TypeKindDomain, Constraints: []*ir.DomainConstraint{{Definition: "CHECK (is_valid(VALUE))"}}},
	}

	now, after := holdBackFunctionsOfDroppedTypes([]*ir.Function{diffFn, other, checkFn}, types)
	if len(now) != 1 || now[0] != other {
		t.Fatalf("expected only the unrelated function to drop now, got %v", now)
	}
	if len(after) != 2 || after[0] != diffFn || after[1] != checkFn {
		t.Fatalf("expected the referenced functions to wait in order, got %v", after)
	}

	now, after = holdBackFunctionsOfDroppedTypes([]*ir.Function{other}, nil)
	if len(now) != 1 || after != nil {
		t.Fatalf("no dropped types must leave the functions untouched")
	}
}
