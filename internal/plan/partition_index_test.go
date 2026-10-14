package plan

import (
	"reflect"
	"strings"
	"testing"

	"github.com/pgplex/pgschema/internal/diff"
	"github.com/pgplex/pgschema/ir"
)

func TestPartitionIndexAttachments(t *testing.T) {
	desired := parseSQL(t, `
CREATE TABLE events (id integer) PARTITION BY RANGE (id);
CREATE TABLE events_early PARTITION OF events FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (id);
CREATE TABLE events_leaf PARTITION OF events_early FOR VALUES FROM (0) TO (5);
CREATE TABLE events_later PARTITION OF events FOR VALUES FROM (10) TO (20);
CREATE INDEX leaf_lookup ON events_leaf (id);
CREATE INDEX later_lookup ON events_later (id);
CREATE INDEX early_lookup ON ONLY events_early (id);
ALTER INDEX early_lookup ATTACH PARTITION leaf_lookup;
CREATE INDEX parent_lookup ON ONLY events (id);
ALTER INDEX parent_lookup ATTACH PARTITION early_lookup;
ALTER INDEX parent_lookup ATTACH PARTITION later_lookup;
`)
	diffs := diff.GenerateMigration(ir.NewIR(), desired, "public")
	p := NewPlan(diffs, 18, nil)
	if len(p.Groups) != 1 {
		t.Fatalf("got %d groups, want one transaction", len(p.Groups))
	}
	var canonical []Step
	var attachments []string
	for _, d := range diffs {
		for _, stmt := range d.Statements {
			canonical = append(canonical, Step{SQL: stmt.SQL, Type: d.Type.String(), Operation: d.Operation.String(), Path: d.Path})
			if d.Operation == diff.DiffOperationAttach {
				attachments = append(attachments, stmt.SQL)
				index, ok := d.Source.(*ir.Index)
				if !ok || d.Path != index.Schema+"."+index.Table+"."+index.Name || d.Type != diff.DiffTypeTableIndex || !stmt.CanRunInTransaction {
					t.Fatalf("invalid attachment metadata: %+v", d)
				}
			}
		}
	}
	wantAttachments := []string{
		"ALTER INDEX early_lookup ATTACH PARTITION leaf_lookup;",
		"ALTER INDEX parent_lookup ATTACH PARTITION early_lookup;",
		"ALTER INDEX parent_lookup ATTACH PARTITION later_lookup;",
	}
	if !reflect.DeepEqual(attachments, wantAttachments) {
		t.Fatalf("attachments = %v, want %v", attachments, wantAttachments)
	}
	if !reflect.DeepEqual(p.Groups[0].Steps, canonical) {
		t.Fatalf("plan changed canonical statements or order: %+v", p.Groups)
	}
	human := p.HumanColored(false)
	for _, name := range []string{"early_lookup", "parent_lookup"} {
		if !strings.Contains(human, name+" (index attachment)") {
			t.Errorf("missing attachment label for %s:\n%s", name, human)
		}
	}
	if strings.Contains(human, "concurrent rebuild") {
		t.Fatalf("attachment labeled as rebuild:\n%s", human)
	}
	summary := p.calculateSummaryFromSteps()
	if summary.Total != 4 || summary.Add != 4 || summary.Change != 0 || summary.Destroy != 0 || summary.ByType["tables"].Add != 4 {
		t.Fatalf("unexpected table counts: %+v", summary)
	}
	checkIndexPlanRoundTrips(t, p)
}

func TestIndexReplacementRetainsConcurrentRewrite(t *testing.T) {
	old := parseSQL(t, `CREATE TABLE users (id integer, name text); CREATE INDEX lookup ON users (id);`)
	desired := parseSQL(t, `CREATE TABLE users (id integer, name text); CREATE INDEX lookup ON users (name);`)
	p := NewPlan(diff.GenerateMigration(old, desired, "public"), 18, old)
	human := p.HumanColored(false)
	if !strings.Contains(human, "lookup (index - concurrent rebuild)") || strings.Contains(human, "index attachment") {
		t.Fatalf("unexpected replacement label:\n%s", human)
	}
	sql := p.ToSQL(SQLFormatRaw)
	if !strings.Contains(sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS lookup_pgschema_new ON users (name);") ||
		!strings.Contains(sql, "DROP INDEX IF EXISTS lookup;") ||
		!strings.Contains(sql, "ALTER INDEX lookup_pgschema_new RENAME TO lookup;") || len(p.Groups) != 3 {
		t.Fatalf("replacement lost concurrent rewrite:\n%s", sql)
	}
	for _, group := range p.Groups {
		for _, step := range group.Steps {
			if step.Operation != "alter" {
				t.Fatalf("replacement operation = %q, want alter", step.Operation)
			}
		}
	}
	checkIndexPlanRoundTrips(t, p)
}

func checkIndexPlanRoundTrips(t *testing.T, p *Plan) {
	t.Helper()
	for _, debug := range []bool{false, true} {
		name := "normal"
		if debug {
			name = "debug"
		}
		t.Run(name, func(t *testing.T) {
			var encoded string
			var err error
			if debug {
				encoded, err = p.ToJSONWithDebug(true)
			} else {
				encoded, err = p.ToJSON()
			}
			if err != nil {
				t.Fatal(err)
			}
			restored, err := FromJSON([]byte(encoded))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(restored.Groups, p.Groups) {
				t.Fatal("JSON round trip changed groups, SQL, directives, or metadata")
			}
			for _, format := range []SQLFormat{SQLFormatRaw, SQLFormatHuman} {
				if restored.ToSQL(format) != p.ToSQL(format) {
					t.Errorf("JSON round trip changed %s SQL", format)
				}
			}
			if !reflect.DeepEqual(restored.calculateSummaryFromSteps(), p.calculateSummaryFromSteps()) {
				t.Fatal("JSON round trip changed summary counts")
			}
			if debug {
				if len(restored.SourceDiffs) != len(p.SourceDiffs) {
					t.Fatal("debug JSON lost source diffs")
				}
				for i, d := range restored.SourceDiffs {
					original := p.SourceDiffs[i]
					if d.Operation != original.Operation || d.Type != original.Type || d.Path != original.Path || !reflect.DeepEqual(d.Statements, original.Statements) {
						t.Fatalf("debug JSON changed source diff %d", i)
					}
				}
			} else if len(restored.SourceDiffs) != 0 {
				t.Fatal("normal JSON unexpectedly includes source diffs")
			}
		})
	}
}

func TestConstraintOwnedPartitionIndexAttachments(t *testing.T) {
	desired := parseSQL(t, `
CREATE TABLE events (id integer PRIMARY KEY, a integer, UNIQUE(id,a)) PARTITION BY RANGE(id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);
ALTER INDEX events_leaf_pkey RENAME TO leaf_identity;
COMMENT ON CONSTRAINT events_pkey ON events IS 'root key';
COMMENT ON CONSTRAINT leaf_identity ON events_leaf IS 'child key';
`)
	diffs := diff.GenerateMigration(ir.NewIR(), desired, "public")
	p := NewPlan(diffs, 18, nil)
	if len(p.Groups) != 1 {
		t.Fatalf("expected one transaction, got %d groups", len(p.Groups))
	}
	var attachments []string
	comments := 0
	for _, d := range diffs {
		for _, stmt := range d.Statements {
			if d.Operation == diff.DiffOperationAttach {
				index, ok := d.Source.(*ir.Index)
				if !ok || d.Type != diff.DiffTypeTableIndex || d.Path != index.Schema+"."+index.Table+"."+index.Name || !index.IsPartitioned || !stmt.CanRunInTransaction {
					t.Fatalf("invalid constraint-owned attachment metadata: %+v", d)
				}
				attachments = append(attachments, stmt.SQL)
			}
			if strings.HasPrefix(stmt.SQL, "COMMENT ON CONSTRAINT") {
				comments++
				if d.Type != diff.DiffTypeTableComment || d.Operation != diff.DiffOperationCreate {
					t.Fatalf("invalid constraint comment metadata: %+v", d)
				}
			}
		}
	}
	want := []string{"ALTER INDEX events_id_a_key ATTACH PARTITION events_leaf_id_a_key;", "ALTER INDEX events_pkey ATTACH PARTITION leaf_identity;"}
	if !reflect.DeepEqual(attachments, want) {
		t.Fatalf("attachments=%v, want %v", attachments, want)
	}
	if comments != 2 {
		t.Fatalf("got %d constraint comments, want 2", comments)
	}
	summary := p.calculateSummaryFromSteps()
	if summary.Total != 2 || summary.Add != 2 || summary.Change != 0 || summary.Destroy != 0 {
		t.Fatalf("comments or attachments changed table counts: %+v", summary)
	}
	human := p.HumanColored(false)
	for _, name := range []string{"events_pkey", "events_id_a_key"} {
		if !strings.Contains(human, name+" (index attachment)") {
			t.Errorf("missing attachment label for %s:\n%s", name, human)
		}
	}
	if strings.Contains(human, "concurrent rebuild") || strings.Contains(p.ToSQL(SQLFormatRaw), "CONCURRENTLY") {
		t.Fatal("new attachments classified as rebuild/concurrent creation")
	}
	checkIndexPlanRoundTrips(t, p)
}
