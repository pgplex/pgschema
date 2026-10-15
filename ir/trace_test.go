package ir

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fakeDBTX is a connection stand-in that answers every call immediately.
type fakeDBTX struct{}

func (fakeDBTX) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}
func (fakeDBTX) PrepareContext(context.Context, string) (*sql.Stmt, error) { return nil, nil }
func (fakeDBTX) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}
func (fakeDBTX) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func TestQueryShapeKey(t *testing.T) {
	cases := map[string]string{
		"-- name: GetPrivilegesForSchema :many\nWITH acl_data AS (...)": "GetPrivilegesForSchema",
		"SELECT rolname FROM pg_roles WHERE oid = $1":                   "SELECT rolname FROM pg_roles WHERE oid = $1",
		"SELECT   version()\n":                                          "SELECT version()",
		"SELECT " + strings.Repeat("x", 100):                            "SELECT " + strings.Repeat("x", 63) + "...",
	}
	for in, want := range cases {
		if got := queryShapeKey(in); got != want {
			t.Errorf("queryShapeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTracingDBAttributesQueriesToSpans(t *testing.T) {
	db := &tracingDB{inner: fakeDBTX{}}
	root := newTraceSpan("root")
	ctx := context.WithValue(context.Background(), traceSpanKey{}, root)

	// A query outside any span is simply not recorded.
	db.QueryRowContext(context.Background(), "SELECT 1")

	// One query directly in the root.
	db.QueryRowContext(ctx, "SELECT version()")

	// A step with one batched query and three per-row lookups.
	err := traced(ctx, "privileges", func(ctx context.Context) error {
		_, _ = db.QueryContext(ctx, "-- name: GetPrivilegesForSchema :many\nSELECT 1")
		for range 3 {
			db.QueryRowContext(ctx, "SELECT rolname FROM pg_roles WHERE oid = $1", 42)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A concurrent group: a light step finishes first, a heavier one second.
	groupCtx, group := startSpan(ctx, "group")
	group.concurrent = true
	err = traced(groupCtx, "partitions", func(ctx context.Context) error {
		_, _ = db.QueryContext(ctx, "-- name: GetPartitionedTablesForSchema :many\nSELECT 1")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = traced(groupCtx, "columns", func(ctx context.Context) error {
		_, _ = db.QueryContext(ctx, "-- name: GetColumnsForSchema :many\nSELECT 1")
		_, _ = db.ExecContext(ctx, "SET x = 1")
		_, _ = db.PrepareContext(ctx, "SELECT 2")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	group.end()
	root.end()

	if root.queries != 1 {
		t.Errorf("root own queries = %d, want 1", root.queries)
	}
	if n, _ := root.totals(); n != 9 {
		t.Errorf("root total queries = %d, want 9", n)
	}
	if len(root.children) != 2 {
		t.Fatalf("root children = %d, want 2", len(root.children))
	}

	priv := root.children[0]
	if priv.name != "privileges" || priv.queries != 4 {
		t.Errorf("privileges span = %q with %d queries, want 4", priv.name, priv.queries)
	}
	if sh := priv.shapes["SELECT rolname FROM pg_roles WHERE oid = $1"]; sh == nil || sh.count != 3 {
		t.Errorf("per-row lookup shape not aggregated: %+v", sh)
	}
	if sh := priv.shapes["GetPrivilegesForSchema"]; sh == nil || sh.count != 1 {
		t.Errorf("sqlc query shape not keyed by name: %+v", sh)
	}

	if root.children[1] != group || group.queries != 0 || len(group.children) != 2 {
		t.Errorf("group span not nested as expected: own=%d children=%d", group.queries, len(group.children))
	}
	if n, _ := group.totals(); n != 4 {
		t.Errorf("group total queries = %d, want 4", n)
	}
	if root.wall <= 0 {
		t.Errorf("root wall time not recorded")
	}

	// The logged tree: one line per node, indented, with the per-shape
	// breakdown only under steps that issued more than one query.
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
	root.log(logger)
	out := buf.String()
	for _, want := range []string{
		`msg="inspector: root" queries=9`,
		`msg="inspector:   privileges" queries=4`,
		`msg="inspector:     SELECT rolname FROM pg_roles WHERE oid = $1" queries=3`,
		`msg="inspector:     GetPrivilegesForSchema" queries=1`,
		`msg="inspector:   group" queries=4`,
		`msg="inspector:     columns" queries=3`,
		`msg="inspector:     partitions" queries=1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q:\n%s", want, out)
		}
	}
	// Children of a concurrent group are logged heaviest first, regardless of
	// which finished first.
	if strings.Index(out, `"inspector:     columns"`) > strings.Index(out, `"inspector:     partitions"`) {
		t.Errorf("concurrent group children not sorted by query count:\n%s", out)
	}
	// The root issued one query, so it gets no shape line of its own.
	if strings.Contains(out, `"inspector:   SELECT version()"`) {
		t.Errorf("single-query span should not be broken down by shape:\n%s", out)
	}
}

func TestTraceSpanConcurrentRecording(t *testing.T) {
	span := newTraceSpan("s")
	done := make(chan struct{})
	for range 8 {
		go func() {
			for range 100 {
				span.record("SELECT 1", time.Microsecond)
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}
	if span.queries != 800 || span.shapes["SELECT 1"].count != 800 {
		t.Errorf("concurrent recording lost updates: queries=%d", span.queries)
	}
}
