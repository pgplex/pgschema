package diff

import (
	"strings"
	"testing"

	"github.com/pgplex/pgschema/ir"
	"github.com/pgplex/pgschema/testutil"
)

// A function whose return type changes is dropped and created again. Objects
// that call it and cannot be restored unchanged around that are rejected at
// plan time with the column and the function named; the ones that can are
// not (#601).
func TestValidateFunctionRecreations(t *testing.T) {
	const oldFunction = "CREATE FUNCTION lim(x integer) RETURNS integer LANGUAGE sql IMMUTABLE AS 'select x';\n"
	const newFunction = "CREATE FUNCTION lim(x integer) RETURNS bigint LANGUAGE sql IMMUTABLE AS 'select x';\n"

	tests := []struct {
		name      string
		oldTables string
		newTables string
		wantErr   []string
	}{
		{
			name:      "generated column",
			oldTables: "CREATE TABLE g (id integer, v integer, d integer GENERATED ALWAYS AS (lim(v)) STORED);",
			newTables: "CREATE TABLE g (id integer, v integer, d integer GENERATED ALWAYS AS (lim(v)) STORED);",
			wantErr:   []string{"generated column public.g.d calls public.lim(integer); keeping it would require dropping and re-adding the column"},
		},
		{
			name:      "generated column on a new table",
			newTables: "CREATE TABLE g (id integer, v integer, d integer GENERATED ALWAYS AS (lim(v)) STORED);",
			wantErr:   []string{"generated column public.g.d of new table public.g calls public.lim(integer)", "create the table in a separate step"},
		},
		{
			name:      "column added with a default",
			oldTables: "CREATE TABLE a (id integer);",
			newTables: "CREATE TABLE a (id integer, n integer DEFAULT lim(1));",
			wantErr:   []string{"column public.a.n is added with a DEFAULT that calls public.lim(integer), so its existing rows would be filled before the function is created again"},
		},
		{
			name:      "column added with a domain whose default calls it",
			oldTables: "CREATE TABLE a (id integer);",
			newTables: "CREATE DOMAIN amount AS integer DEFAULT lim(1); CREATE DOMAIN positive_amount AS amount CHECK (VALUE > 0); CREATE TABLE a (id integer, n amount, p positive_amount, q amount DEFAULT 0, r amount[]);",
			wantErr: []string{
				"column public.a.n is added with type public.amount, whose DEFAULT calls public.lim(integer)",
				"column public.a.p is added with type public.positive_amount, whose DEFAULT calls public.lim(integer)",
			},
		},
		{
			name:      "column type change with a new default",
			oldTables: "CREATE TABLE c (id integer, v text DEFAULT lower('X'));",
			newTables: "CREATE TABLE c (id integer, v integer DEFAULT lim(1));",
			wantErr:   []string{"column public.c.v changes its type or collation and gets a DEFAULT that calls public.lim(integer); change the column type or collation in a separate step"},
		},
		{
			name:      "nullability change with a new default",
			oldTables: "CREATE TABLE c (id integer, v integer DEFAULT 0);",
			newTables: "CREATE TABLE c (id integer, v integer NOT NULL DEFAULT lim(1));",
		},
		{
			name:      "default on an existing column",
			oldTables: "CREATE TABLE c (id integer, n integer DEFAULT lim(1));",
			newTables: "CREATE TABLE c (id integer, n integer DEFAULT lim(1));",
		},
		{
			name:      "new table with a default",
			newTables: "CREATE TABLE c (id integer, n integer DEFAULT lim(1));",
		},
		{
			name:      "generated column that stops calling the function",
			oldTables: "CREATE TABLE g (id integer, v integer, d integer GENERATED ALWAYS AS (lim(v)) STORED);",
			newTables: "CREATE TABLE g (id integer, v integer, d integer GENERATED ALWAYS AS (v * 2) STORED);",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, oldFunction+tt.oldTables, "public", "")
			newIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, newFunction+tt.newTables, "public", "")
			err := ValidateFunctionRecreations(oldIR, newIR, 0)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q", tt.wantErr)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err.Error(), want)
				}
			}
		})
	}

	// No function is recreated: nothing to reject.
	oldIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, oldFunction+"CREATE TABLE a (id integer);", "public", "")
	newIR := testutil.ParseSQLToIRWithSetup(t, sharedTestPostgres, oldFunction+"CREATE TABLE a (id integer, n integer DEFAULT lim(1));", "public", "")
	if err := ValidateFunctionRecreations(oldIR, newIR, 0); err != nil {
		t.Fatalf("unexpected error without a recreated function: %v", err)
	}
}

// A VIRTUAL generated column turned into a plain column is re-created (DROP +
// ADD COLUMN, #591); with a DEFAULT calling a recreated function its rows
// would be filled by the old function, so the plan is refused. A STORED one
// keeps its values (DROP EXPRESSION) and its default is held instead. The IR
// is built by hand because VIRTUAL columns need PostgreSQL 18.
func TestValidateFunctionRecreations_RecreatedColumnDefault(t *testing.T) {
	schemaIR := func(returnType string, column *ir.Column) *ir.IR {
		return &ir.IR{Schemas: map[string]*ir.Schema{"public": {
			Name: "public",
			Functions: map[string]*ir.Function{"lim()": {
				Schema: "public", Name: "lim", ReturnType: returnType, Language: "sql", Definition: "select 1",
			}},
			Tables: map[string]*ir.Table{"t": {
				Schema: "public", Name: "t", Type: ir.TableTypeBase,
				Columns: []*ir.Column{{Name: "id", Position: 1, DataType: "integer"}, column},
			}},
		}}}
	}
	generated := func(kind string) *ir.Column {
		expr := "(id * 2)"
		return &ir.Column{Name: "n", Position: 2, DataType: "integer", IsNullable: true, IsGenerated: true, GeneratedKind: kind, GeneratedExpr: &expr}
	}
	defaultValue := "lim()"
	plain := &ir.Column{Name: "n", Position: 2, DataType: "integer", IsNullable: true, DefaultValue: &defaultValue}

	err := ValidateFunctionRecreations(schemaIR("integer", generated("v")), schemaIR("bigint", plain), 18)
	if err == nil || !strings.Contains(err.Error(), "column public.t.n is re-created (DROP + ADD COLUMN) with a DEFAULT that calls public.lim()") {
		t.Fatalf("VIRTUAL -> plain: got %v", err)
	}
	if err := ValidateFunctionRecreations(schemaIR("integer", generated("s")), schemaIR("bigint", plain), 18); err != nil {
		t.Fatalf("STORED -> plain: unexpected error: %v", err)
	}
}
