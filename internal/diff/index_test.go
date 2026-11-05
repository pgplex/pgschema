package diff

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pgplex/pgschema/ir"
)

func TestOnlineIndexTempNameFitsTheIdentifierLimit(t *testing.T) {
	long := strings.Repeat("i", maxIdentifierBytes)
	almost := strings.Repeat("j", maxIdentifierBytes-1)

	tests := []struct {
		name string
		want string // "" means: only the invariants below
	}{
		{name: "lookup", want: "lookup_pgschema_new"},
		{name: long},
		{name: almost},
	}
	for _, tt := range tests {
		got := OnlineIndexTempName(tt.name)
		if tt.want != "" && got != tt.want {
			t.Errorf("OnlineIndexTempName(%q) = %q, want %q", tt.name, got, tt.want)
		}
		if len(got) > maxIdentifierBytes {
			t.Errorf("OnlineIndexTempName(%q) is %d bytes, over the %d-byte limit: %q", tt.name, len(got), maxIdentifierBytes, got)
		}
		if got == tt.name {
			t.Errorf("OnlineIndexTempName(%q) = the canonical name: the candidate would be truncated to it", tt.name)
		}
		if !strings.HasSuffix(got, OnlineIndexTempNameSuffix) {
			t.Errorf("OnlineIndexTempName(%q) = %q, missing the %q suffix", tt.name, got, OnlineIndexTempNameSuffix)
		}
	}

	// Two names that share a long prefix still get distinct candidates.
	first := OnlineIndexTempName(strings.Repeat("k", maxIdentifierBytes-1) + "a")
	second := OnlineIndexTempName(strings.Repeat("k", maxIdentifierBytes-1) + "b")
	if first == second {
		t.Errorf("two long names share the candidate %q", first)
	}

	// A name with multi-byte characters is cut on a rune boundary.
	utf8Name := strings.Repeat("é", maxIdentifierBytes)
	got := OnlineIndexTempName(utf8Name)
	if !utf8.ValidString(got) {
		t.Errorf("onlineIndexTempName split a rune: %q", got)
	}
	if len(got) > maxIdentifierBytes {
		t.Errorf("OnlineIndexTempName(%q) is %d bytes, over the limit", utf8Name, len(got))
	}
}

// issue672Index builds an index on entries(column) for the candidate-reuse tests.
func issue672Index(name, column string, valid bool) *ir.Index {
	return &ir.Index{
		Schema:  "public",
		Table:   "entries",
		Name:    name,
		Type:    ir.IndexTypeRegular,
		Method:  "btree",
		IsValid: valid,
		Columns: []*ir.IndexColumn{{Name: column, Position: 1}},
	}
}

// TestOnlineIndexCandidateReuse covers the cases the file-based fixtures cannot
// set up: a candidate that PostgreSQL left invalid, and one on a column that
// the same plan drops and re-creates. Both must be rebuilt, not adopted. (#672)
func TestOnlineIndexCandidateReuse(t *testing.T) {
	const canonical = "entries_sort_idx"
	candidateName := OnlineIndexTempName(canonical)

	tests := []struct {
		name             string
		candidate        *ir.Index
		recreatedColumns map[string]bool
		wantReuse        bool
	}{
		{
			name:      "valid matching candidate is renamed into place",
			candidate: issue672Index(candidateName, "b", true),
			wantReuse: true,
		},
		{
			name:      "invalid candidate is dropped and rebuilt",
			candidate: issue672Index(candidateName, "b", false),
			wantReuse: false,
		},
		{
			name:             "candidate on a re-created column is dropped and rebuilt",
			candidate:        issue672Index(candidateName, "b", true),
			recreatedColumns: map[string]bool{"b": true},
			wantReuse:        false,
		},
		{
			name:      "candidate on another table is dropped and rebuilt",
			candidate: func() *ir.Index { i := issue672Index(candidateName, "b", true); i.Table = "other"; return i }(),
			wantReuse: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := newDiffCollector()
			generateIndexModifications(
				[]*ir.Index{issue672Index(canonical, "a", true), tt.candidate},
				[]*ir.Index{issue672Index(canonical, "b", true)},
				nil,
				tt.recreatedColumns,
				"public",
				DiffTypeTableIndex,
				DiffTypeTableIndexComment,
				collector,
			)
			sql := buildSQLFromSteps(collector.diffs)

			rename := "ALTER INDEX " + candidateName + " RENAME TO " + canonical + ";"
			dropCandidate := "DROP INDEX IF EXISTS " + candidateName + ";"
			create := "CREATE INDEX IF NOT EXISTS " + canonical + " ON entries (b);"
			if tt.wantReuse {
				if !strings.Contains(sql, rename) || strings.Contains(sql, dropCandidate) || strings.Contains(sql, create) {
					t.Errorf("expected the candidate to be renamed into place, got:\n%s", sql)
				}
				for _, d := range collector.diffs {
					if id, ok := d.Source.(*IndexDiff); ok && id.ReusedCandidate != candidateName {
						t.Errorf("ReusedCandidate = %q, want %q", id.ReusedCandidate, candidateName)
					}
				}
				return
			}
			if strings.Contains(sql, rename) || !strings.Contains(sql, dropCandidate) || !strings.Contains(sql, create) {
				t.Errorf("expected the candidate to be dropped and the index rebuilt, got:\n%s", sql)
			}
		})
	}
}

// TestOnlineIndexCandidateReuseRejectsAttachedAndSharedCandidates covers two
// candidates that must never be adopted: one attached to a parent index, and
// one whose name two replacements both claim. (#672)
func TestOnlineIndexCandidateReuseRejectsAttachedAndSharedCandidates(t *testing.T) {
	t.Run("attached candidate is dropped and rebuilt", func(t *testing.T) {
		const canonical = "entries_sort_idx"
		candidate := issue672Index(OnlineIndexTempName(canonical), "b", true)
		candidate.ParentIndex = "entries_parent_idx"
		candidate.ParentIndexSchema = "public"

		collector := newDiffCollector()
		generateIndexModifications(
			[]*ir.Index{issue672Index(canonical, "a", true), candidate},
			[]*ir.Index{issue672Index(canonical, "b", true)},
			nil, nil, "public", DiffTypeTableIndex, DiffTypeTableIndexComment, collector,
		)
		sql := buildSQLFromSteps(collector.diffs)
		if strings.Contains(sql, "RENAME TO") || !strings.Contains(sql, "DROP INDEX IF EXISTS "+candidate.Name+";") {
			t.Errorf("attached candidate must be dropped and rebuilt, got:\n%s", sql)
		}
	})

	t.Run("partitioned candidate is dropped and rebuilt", func(t *testing.T) {
		const canonical = "entries_sort_idx"
		candidate := issue672Index(OnlineIndexTempName(canonical), "b", true)
		candidate.IsPartitioned = true

		collector := newDiffCollector()
		generateIndexModifications(
			[]*ir.Index{issue672Index(canonical, "a", true), candidate},
			[]*ir.Index{issue672Index(canonical, "b", true)},
			nil, nil, "public", DiffTypeTableIndex, DiffTypeTableIndexComment, collector,
		)
		sql := buildSQLFromSteps(collector.diffs)
		if strings.Contains(sql, "RENAME TO") || !strings.Contains(sql, "DROP INDEX IF EXISTS "+candidate.Name+";") {
			t.Errorf("partitioned candidate must be dropped and rebuilt, got:\n%s", sql)
		}
	})

	t.Run("candidate name claimed by two replacements is not adopted", func(t *testing.T) {
		// A 63-byte name gets a digest-form candidate; the name equal to that
		// candidate minus the suffix gets the very same candidate in plain form.
		long := strings.Repeat("i", maxIdentifierBytes)
		candidateName := OnlineIndexTempName(long)
		short := strings.TrimSuffix(candidateName, OnlineIndexTempNameSuffix)
		if OnlineIndexTempName(short) != candidateName {
			t.Fatalf("test setup: %q and %q do not share a candidate", long, short)
		}

		collector := newDiffCollector()
		generateIndexModifications(
			[]*ir.Index{issue672Index(long, "a", true), issue672Index(short, "a", true), issue672Index(candidateName, "b", true)},
			[]*ir.Index{issue672Index(long, "b", true), issue672Index(short, "b", true)},
			nil, nil, "public", DiffTypeTableIndex, DiffTypeTableIndexComment, collector,
		)
		sql := buildSQLFromSteps(collector.diffs)
		if strings.Contains(sql, "RENAME TO") || !strings.Contains(sql, "DROP INDEX IF EXISTS "+ir.QuoteIdentifier(candidateName)+";") {
			t.Errorf("shared candidate must be dropped and both indexes rebuilt, got:\n%s", sql)
		}
	})
}
