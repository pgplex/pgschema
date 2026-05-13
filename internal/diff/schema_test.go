package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
)

func TestGenerateCreateSchemasSQL_includesOwner(t *testing.T) {
	c := newDiffCollector()
	generateCreateSchemasSQL([]*ir.Schema{{Name: "app", Owner: "postgres"}}, c)
	if len(c.diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(c.diffs))
	}
	sql := c.diffs[0].Statements[0].SQL
	if !strings.Contains(sql, "CREATE SCHEMA IF NOT EXISTS") || !strings.Contains(sql, `"app"`) {
		t.Fatalf("unexpected sql: %s", sql)
	}
	if !strings.Contains(sql, "AUTHORIZATION") || !strings.Contains(sql, `"postgres"`) {
		t.Fatalf("expected AUTHORIZATION, got: %s", sql)
	}
}

func TestGenerateDropSchemasSQL_usesCascade(t *testing.T) {
	c := newDiffCollector()
	generateDropSchemasSQL([]*ir.Schema{{Name: "app"}}, c)
	if len(c.diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(c.diffs))
	}
	sql := c.diffs[0].Statements[0].SQL
	if !strings.Contains(sql, "DROP SCHEMA IF EXISTS") || !strings.Contains(sql, "CASCADE") {
		t.Fatalf("unexpected sql: %s", sql)
	}
}
