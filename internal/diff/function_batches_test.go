package diff

import (
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestHoldBackCallersAcrossBatches(t *testing.T) {
	fn := func(name, language, body string) *ir.Function {
		return &ir.Function{Schema: "public", Name: name, Language: language, Definition: body}
	}
	names := func(fns []*ir.Function) []string {
		var out []string
		for _, f := range fns {
			out = append(out, f.Name)
		}
		return out
	}

	// caller (second batch) -> helper (first batch) -> last (third batch): helper only
	// joins the third batch after caller was first examined, so caller needs another round.
	helper := fn("helper", "sql", "SELECT last()")
	independent := fn("independent", "sql", "SELECT 1")
	dynamic := fn("dynamic", "plpgsql", "BEGIN RETURN last(); END")
	caller := fn("caller", "sql", "SELECT helper() FROM t")
	last := fn("last", "sql", "SELECT count(*) FROM t2")

	first, second, third := holdBackCallersAcrossBatches(
		[]*ir.Function{helper, independent, dynamic},
		[]*ir.Function{caller},
		[]*ir.Function{last},
	)

	assertNames := func(label string, got []*ir.Function, want ...string) {
		t.Helper()
		g := names(got)
		if len(g) != len(want) {
			t.Fatalf("%s: got %v, want %v", label, g, want)
		}
		for i := range want {
			if g[i] != want[i] {
				t.Fatalf("%s: got %v, want %v", label, g, want)
			}
		}
	}
	// plpgsql resolves calls at run time and is never held back.
	assertNames("first", first, "independent", "dynamic")
	assertNames("second", second)
	assertNames("third", third, "last", "helper", "caller")
}
