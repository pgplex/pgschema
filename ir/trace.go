package ir

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pgplex/pgschema/ir/queries"
)

// Query tracing for the inspector.
//
// Every catalog query the inspector issues goes through tracingDB, which
// attributes it to the traceSpan carried by the calling context: one span per
// BuildIR step ("tables", "constraints", "privileges", ...), nested under its
// concurrent group and the BuildIR root. With --debug the finished tree is
// logged when BuildIR returns, one line per node with its query count and
// time, and each step with more than one query is broken down by query shape.
// Over a high-latency connection that shows directly which step - and which
// per-row lookup inside it - is paying for the round trips (issues #285,
// #644, #654), without needing server-side statement logging.

type traceSpanKey struct{}

// traceSpan is one node of the inspector's query call tree.
type traceSpan struct {
	name  string
	start time.Time
	wall  time.Duration
	// concurrent marks a span whose children ran in parallel. Their order of
	// completion carries no meaning, so they are logged heaviest first.
	concurrent bool

	mu        sync.Mutex
	children  []*traceSpan
	queries   int           // queries issued directly in this span
	queryTime time.Duration // time spent in those queries
	shapes    map[string]*queryShape
}

// queryShape aggregates the queries of one span that share a shape key.
type queryShape struct {
	key   string
	count int
	time  time.Duration
}

func newTraceSpan(name string) *traceSpan {
	return &traceSpan{name: name, start: time.Now(), shapes: map[string]*queryShape{}}
}

// startSpan opens a child span under the span carried by ctx, if any, and
// returns a context carrying the child.
func startSpan(ctx context.Context, name string) (context.Context, *traceSpan) {
	child := newTraceSpan(name)
	if parent := spanFromContext(ctx); parent != nil {
		parent.mu.Lock()
		parent.children = append(parent.children, child)
		parent.mu.Unlock()
	}
	return context.WithValue(ctx, traceSpanKey{}, child), child
}

func spanFromContext(ctx context.Context) *traceSpan {
	s, _ := ctx.Value(traceSpanKey{}).(*traceSpan)
	return s
}

// traced runs fn inside a new child span named name.
func traced(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := startSpan(ctx, name)
	defer span.end()
	return fn(ctx)
}

func (s *traceSpan) end() { s.wall = time.Since(s.start) }

func (s *traceSpan) record(sqlText string, d time.Duration) {
	key := queryShapeKey(sqlText)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queries++
	s.queryTime += d
	shape := s.shapes[key]
	if shape == nil {
		shape = &queryShape{key: key}
		s.shapes[key] = shape
	}
	shape.count++
	shape.time += d
}

// totals returns the query count and query time of this span including all
// descendants.
func (s *traceSpan) totals() (int, time.Duration) {
	s.mu.Lock()
	n, d := s.queries, s.queryTime
	children := append([]*traceSpan(nil), s.children...)
	s.mu.Unlock()
	for _, c := range children {
		cn, cd := c.totals()
		n += cn
		d += cd
	}
	return n, d
}

// log writes the span tree at debug level, one line per node, indented by
// depth. Every line starts with "inspector:" so the tree can be grepped out
// of the rest of the debug output.
func (s *traceSpan) log(log *slog.Logger) {
	s.logNode(log, 0)
}

func (s *traceSpan) logNode(log *slog.Logger, depth int) {
	n, d := s.totals()
	attrs := []any{"queries", n, "query_time", d.Round(time.Millisecond)}
	if s.wall > 0 {
		attrs = append(attrs, "wall", s.wall.Round(time.Millisecond))
	}
	log.Debug("inspector: "+strings.Repeat("  ", depth)+s.name, attrs...)

	s.mu.Lock()
	children := append([]*traceSpan(nil), s.children...)
	shapes := make([]*queryShape, 0, len(s.shapes))
	for _, sh := range s.shapes {
		shapes = append(shapes, sh)
	}
	own := s.queries
	s.mu.Unlock()

	if s.concurrent {
		sort.SliceStable(children, func(a, b int) bool {
			na, _ := children[a].totals()
			nb, _ := children[b].totals()
			return na > nb
		})
	}
	for _, c := range children {
		c.logNode(log, depth+1)
	}

	// A step that issues a single query needs no breakdown. One that issues
	// several is split by shape, so a per-row lookup appears with its count
	// next to the batched query it accompanies.
	if own > 1 {
		sort.Slice(shapes, func(a, b int) bool {
			if shapes[a].count != shapes[b].count {
				return shapes[a].count > shapes[b].count
			}
			return shapes[a].key < shapes[b].key
		})
		for _, sh := range shapes {
			log.Debug("inspector: "+strings.Repeat("  ", depth+1)+sh.key,
				"queries", sh.count, "query_time", sh.time.Round(time.Millisecond))
		}
	}
}

// queryShapeKey reduces a SQL text to a short label: the sqlc query name when
// the text starts with sqlc's "-- name:" header, otherwise the leading part of
// the statement with whitespace collapsed.
func queryShapeKey(sqlText string) string {
	const header = "-- name: "
	if strings.HasPrefix(sqlText, header) {
		rest := sqlText[len(header):]
		if i := strings.IndexAny(rest, " \n"); i > 0 {
			return rest[:i]
		}
	}
	collapsed := strings.Join(strings.Fields(sqlText), " ")
	const maxLen = 70
	if len(collapsed) > maxLen {
		return collapsed[:maxLen] + "..."
	}
	return collapsed
}

// tracingDB wraps the inspector's connection and records every call against
// the span in the calling context. The recorded time covers the call itself -
// sending the statement and receiving the first response - not the later
// reading of result rows, so for the inspector's single-row and small-result
// queries it is close to one network round trip.
type tracingDB struct {
	inner queries.DBTX
}

var _ queries.DBTX = (*tracingDB)(nil)

func (t *tracingDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	start := time.Now()
	res, err := t.inner.ExecContext(ctx, query, args...)
	t.record(ctx, query, start)
	return res, err
}

func (t *tracingDB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := t.inner.PrepareContext(ctx, query)
	t.record(ctx, query, start)
	return stmt, err
}

func (t *tracingDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	start := time.Now()
	rows, err := t.inner.QueryContext(ctx, query, args...)
	t.record(ctx, query, start)
	return rows, err
}

func (t *tracingDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	start := time.Now()
	row := t.inner.QueryRowContext(ctx, query, args...)
	t.record(ctx, query, start)
	return row
}

func (t *tracingDB) record(ctx context.Context, query string, start time.Time) {
	if s := spanFromContext(ctx); s != nil {
		s.record(query, time.Since(start))
	}
}
