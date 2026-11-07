package diff

import (
	"slices"
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

	// Without "range" in the name the server clips the range name to
	// NAMEDATALEN-12 bytes and keeps the whole suffix.
	long := strings.Repeat("a", 60) + "span"
	if got, want := defaultMultirangeTypeName(long), strings.Repeat("a", 52)+"_multirange"; got != want {
		t.Errorf("long name: got %q, want %q", got, want)
	}
	// With "range" in the name the combined name is clipped at the end.
	longRange := strings.Repeat("b", 55) + "range"
	if got, want := defaultMultirangeTypeName(longRange), strings.Repeat("b", 55)+"multiran"; got != want {
		t.Errorf("long range name: got %q, want %q", got, want)
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
			name: "opclass and collation in the target schema lose the prefix",
			typ: &ir.Type{
				Schema: "public", Name: "textrange", Kind: ir.TypeKindRange,
				Subtype: "text", MultirangeTypeName: "textmultirange",
				SubtypeOpclass: "public.my_text_ops", Collation: "public.my_coll",
			},
			want: "CREATE TYPE textrange AS RANGE (\n    subtype = text,\n    subtype_opclass = my_text_ops,\n    collation = my_coll\n);",
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
	timeParam := &ir.Parameter{Mode: "IN", DataType: "time without time zone"}
	diffFn := &ir.Function{Schema: "public", Name: "time_diff", Parameters: []*ir.Parameter{timeParam, timeParam}}
	checkFn := &ir.Function{Schema: "public", Name: "is_valid"}
	other := &ir.Function{Schema: "public", Name: "unrelated"}
	types := []*ir.Type{
		{Schema: "public", Name: "timerange", Kind: ir.TypeKindRange, Subtype: "time without time zone", SubtypeDiff: "time_diff"},
		{Schema: "public", Name: "code", Kind: ir.TypeKindDomain, Constraints: []*ir.DomainConstraint{{Definition: "CHECK (is_valid(VALUE))"}}},
	}

	now, after := holdBackFunctionsOfDroppedTypes([]*ir.Function{diffFn, other, checkFn}, types)
	if len(now) != 1 || now[0] != other {
		t.Fatalf("expected only the unrelated function to drop now, got %v", now)
	}
	if len(after) != 2 || after[0] != diffFn || after[1] != checkFn {
		t.Fatalf("expected the referenced functions to wait in order, got %v", after)
	}

	// The subtype keeps the format_type spelling while parameter types are
	// normalized to the short one; they must still match.
	tsParam := &ir.Parameter{Mode: "IN", DataType: "timestamp"}
	tsDiff := &ir.Function{Schema: "public", Name: "ts_diff", Parameters: []*ir.Parameter{tsParam, tsParam}}
	tsRange := &ir.Type{Schema: "public", Name: "stamp_span", Kind: ir.TypeKindRange, Subtype: "timestamp without time zone", SubtypeDiff: "ts_diff"}
	if now, _ := holdBackFunctionsOfDroppedTypes([]*ir.Function{tsDiff}, []*ir.Type{tsRange}); len(now) != 0 {
		t.Fatalf("timestamp subtype must match the normalized parameter type")
	}

	// Same name, different signature: an overload taking the range is not
	// its subtype_diff and is not held.
	overload := &ir.Function{Schema: "public", Name: "time_diff", Parameters: []*ir.Parameter{{Mode: "IN", DataType: "timerange"}}}
	now, _ = holdBackFunctionsOfDroppedTypes([]*ir.Function{overload}, types[:1])
	if len(now) != 1 {
		t.Fatalf("overload taking the range must not be held as its subtype_diff")
	}

	now, after = holdBackFunctionsOfDroppedTypes([]*ir.Function{other}, nil)
	if len(now) != 1 || after != nil {
		t.Fatalf("no dropped types must leave the functions untouched")
	}
}

func TestDependentTypes(t *testing.T) {
	period := &ir.Type{Schema: "public", Name: "period", Kind: ir.TypeKindRange, Subtype: "date", MultirangeTypeName: "period_multirange"}
	booking := &ir.Type{Schema: "public", Name: "booking", Kind: ir.TypeKindComposite, Columns: []*ir.TypeColumn{{Name: "slot", DataType: "public.period"}}}
	availability := &ir.Type{Schema: "public", Name: "availability", Kind: ir.TypeKindDomain, BaseType: "public.period_multirange"}
	window := &ir.Type{Schema: "public", Name: "window", Kind: ir.TypeKindRange, Subtype: "public.availability", MultirangeTypeName: "window_multirange"}
	unrelated := &ir.Type{Schema: "public", Name: "color", Kind: ir.TypeKindEnum, EnumValues: []string{"red"}}
	all := []*ir.Type{availability, booking, unrelated, period, window}

	got := dependentTypes(all, []*ir.Type{period})
	want := []*ir.Type{availability, booking, window} // in the order of all, seeds excluded
	if len(got) != len(want) {
		t.Fatalf("got %d dependents, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dependent %d: got %s, want %s", i, got[i].Name, want[i].Name)
		}
	}
	if dependentTypes(all, nil) != nil {
		t.Fatal("no seeds must give no dependents")
	}
}

func TestOrderTypeAndFunctionDrops(t *testing.T) {
	isPositive := &ir.Function{Schema: "public", Name: "is_positive", Parameters: []*ir.Parameter{{Mode: "IN", DataType: "integer"}}}
	positive := &ir.Type{Schema: "public", Name: "positive", Kind: ir.TypeKindDomain, BaseType: "integer", Constraints: []*ir.DomainConstraint{{Definition: "CHECK (is_positive(VALUE))"}}}
	positiveDiff := &ir.Function{Schema: "public", Name: "positive_diff", Parameters: []*ir.Parameter{{Mode: "IN", DataType: "positive"}, {Mode: "IN", DataType: "positive"}}}
	span := &ir.Type{Schema: "public", Name: "positive_span", Kind: ir.TypeKindRange, Subtype: "public.positive", SubtypeDiff: "positive_diff", MultirangeTypeName: "positive_span_multirange"}
	spanSet := &ir.Type{Schema: "public", Name: "busy", Kind: ir.TypeKindDomain, BaseType: "public.positive_span_multirange"}

	var names []string
	for _, node := range orderTypeAndFunctionDrops([]*ir.Type{positive, span, spanSet}, []*ir.Function{isPositive, positiveDiff}) {
		if node.function != nil {
			names = append(names, node.function.Name)
		} else {
			names = append(names, node.typeObj.Name)
		}
	}
	want := []string{"busy", "positive_span", "positive_diff", "positive", "is_positive"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", names, want)
	}

	// A held function that the catalog records as depending on a type (a
	// SQL-standard body cast) drops before that type even though no type
	// references it through a signature.
	status := &ir.Type{Schema: "public", Name: "z_status", Kind: ir.TypeKindEnum}
	bodyFn := &ir.Function{Schema: "public", Name: "span_diff", TypeDependencies: []string{"public.z_status"}, Parameters: []*ir.Parameter{{Mode: "IN", DataType: "integer"}, {Mode: "IN", DataType: "integer"}}}
	intSpan := &ir.Type{Schema: "public", Name: "span", Kind: ir.TypeKindRange, Subtype: "integer", SubtypeDiff: "span_diff", MultirangeTypeName: "span_multirange"}
	names = nil
	for _, node := range orderTypeAndFunctionDrops([]*ir.Type{status, intSpan}, []*ir.Function{bodyFn}) {
		if node.function != nil {
			names = append(names, node.function.Name)
		} else {
			names = append(names, node.typeObj.Name)
		}
	}
	if strings.Join(names, ",") != "span,span_diff,z_status" {
		t.Fatalf("got %v", names)
	}

	// Two ranges whose difference functions cross-reference: a_diff's body
	// only mentions z_span in a literal (no recorded dependency), z_diff's
	// body casts to a_span (recorded). z_diff must drop before a_span.
	aDiff := &ir.Function{Schema: "public", Name: "a_diff", Definition: "SELECT 'z_span'", Parameters: []*ir.Parameter{{Mode: "IN", DataType: "integer"}, {Mode: "IN", DataType: "integer"}}}
	zDiff := &ir.Function{Schema: "public", Name: "z_diff", TypeDependencies: []string{"public.a_span"}, Parameters: []*ir.Parameter{{Mode: "IN", DataType: "integer"}, {Mode: "IN", DataType: "integer"}}}
	aSpan := &ir.Type{Schema: "public", Name: "a_span", Kind: ir.TypeKindRange, Subtype: "integer", SubtypeDiff: "a_diff", MultirangeTypeName: "a_span_multirange"}
	zSpan := &ir.Type{Schema: "public", Name: "z_span", Kind: ir.TypeKindRange, Subtype: "integer", SubtypeDiff: "z_diff", MultirangeTypeName: "z_span_multirange"}
	names = nil
	for _, node := range orderTypeAndFunctionDrops([]*ir.Type{aSpan, zSpan}, []*ir.Function{aDiff, zDiff}) {
		if node.function != nil {
			names = append(names, node.function.Name)
		} else {
			names = append(names, node.typeObj.Name)
		}
	}
	if slices.Index(names, "z_diff") > slices.Index(names, "a_span") || slices.Index(names, "a_span") > slices.Index(names, "a_diff") || slices.Index(names, "z_span") > slices.Index(names, "z_diff") {
		t.Fatalf("got %v", names)
	}

	// No held functions: plain reverse dependency order of the types.
	names = nil
	for _, node := range orderTypeAndFunctionDrops([]*ir.Type{positive, span}, nil) {
		names = append(names, node.typeObj.Name)
	}
	if strings.Join(names, ",") != "positive_span,positive" {
		t.Fatalf("got %v", names)
	}
}

func TestGenerateAlterCompositeStatements(t *testing.T) {
	oldType := &ir.Type{
		Schema: "public", Name: "address", Kind: ir.TypeKindComposite,
		Columns: []*ir.TypeColumn{
			{Name: "street", DataType: "text"},
			{Name: "zip", DataType: "integer"},
			{Name: "country", DataType: "text"},
		},
	}
	newType := &ir.Type{
		Schema: "public", Name: "address", Kind: ir.TypeKindComposite,
		Columns: []*ir.TypeColumn{
			{Name: "zip", DataType: "bigint"},
			{Name: "street", DataType: "text"},
			{Name: "city", DataType: "text"},
			{Name: "region", DataType: "public.region_kind"},
		},
	}
	got := generateAlterCompositeStatements(oldType, newType, "public", false)
	want := []string{
		"ALTER TYPE address DROP ATTRIBUTE country;",
		"ALTER TYPE address ALTER ATTRIBUTE zip TYPE bigint;",
		"ALTER TYPE address ADD ATTRIBUTE city text;",
		"ALTER TYPE address ADD ATTRIBUTE region region_kind;",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d statements, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d:\n got: %q\nwant: %q", i, got[i], want[i])
		}
	}
}

func TestTypesEqualComposite(t *testing.T) {
	composite := func(cols ...*ir.TypeColumn) *ir.Type {
		return &ir.Type{Schema: "public", Name: "c", Kind: ir.TypeKindComposite, Columns: cols}
	}
	a, b := &ir.TypeColumn{Name: "a", DataType: "integer"}, &ir.TypeColumn{Name: "b", DataType: "text"}
	if !typesEqual(composite(a, b), composite(a, b)) {
		t.Error("identical composites compare unequal")
	}
	// ADD ATTRIBUTE can only append, so attribute order alone is not a change.
	if !typesEqual(composite(a, b), composite(b, a)) {
		t.Error("reordered attributes reported as a change")
	}
	if typesEqual(composite(a), composite(a, b)) {
		t.Error("added attribute not detected")
	}
	if typesEqual(composite(a, b), composite(a, &ir.TypeColumn{Name: "b", DataType: "varchar(10)"})) {
		t.Error("changed attribute type not detected")
	}
	if typesEqual(composite(a, b), composite(a, &ir.TypeColumn{Name: "c", DataType: "text"})) {
		t.Error("renamed attribute not detected")
	}
}
