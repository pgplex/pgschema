# Design: Range type (`CREATE TYPE ... AS RANGE`) support

## Overview

User-defined range types are the remaining `❌` row for pgschema in the
pg-delta comparison post (`docs/blog/supabase-declarative-schema-vs-pgschema.mdx`).
This document scopes the work: what pgschema does with a range type today,
the catalog facts the design rests on, the IR and DDL representation, how a
range type slots into the existing dependency ordering, and the phases.

Domains are the model. A range type is a named type with a handful of scalar
attributes, some of which reference other objects (its subtype, an operator
class, a collation, up to two functions). Everything a domain needs already
exists: a dedicated catalog query, `Type` fields keyed by `Kind`, a
`CREATE`/`DROP` renderer, a topological edge to the type it is built on, and a
"create after functions" bucket for the ones that name a new function. Most of
the work is a fourth `TypeKind` wired through those same places, plus two bug
fixes that bite today whether or not the type itself is managed.

## What happens today

Probe (PostgreSQL 18.1, local): schema `rprobe` with

```sql
CREATE FUNCTION time_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;
CREATE TYPE timerange AS RANGE (subtype = time, subtype_diff = time_diff);
CREATE TYPE floatspan AS RANGE (subtype = float8, multirange_type_name = floatspans);
CREATE TYPE rother.textrange AS RANGE (subtype = text, collation = "C");
CREATE TABLE t (
    id int PRIMARY KEY,
    tr timerange, fs floatspan, fms floatspans, ot rother.textrange,
    b int4range, bm int4multirange,
    EXCLUDE USING gist (tr WITH &&)
);
```

`pgschema dump --schema rprobe` produces the table, `time_diff`, and then
**ten** functions pgschema did not write:

```sql
CREATE TABLE IF NOT EXISTS t (
    id integer,
    tr timerange,
    fs floatspan,
    fms floatspans,
    ot textrange,          -- was rother.textrange
    b int4range,
    bm int4multirange,
    ...
);

CREATE OR REPLACE FUNCTION floatspan(double precision, double precision)
RETURNS floatspan LANGUAGE internal IMMUTABLE PARALLEL SAFE
AS $$range_constructor2$$;
-- ... floatspan/3, floatspans/0, floatspans/1, floatspans/2 (VARIADIC),
--     timerange/2, timerange/3, timemultirange/0, /1, /2
```

Three problems:

1. **The type is missing.** No `CREATE TYPE ... AS RANGE` is emitted, so a
   `plan` from this file against an empty schema produces the `CREATE TABLE`
   as-is and `apply` fails with `ERROR: type "timerange" does not exist`.
2. **Auto-generated constructors leak into the dump.** Every range type
   creates two range constructors and three multirange constructors in
   `pg_proc` (`LANGUAGE internal`). pgschema lists them as user functions.
   They cannot be recreated by a non-superuser, and even a superuser gets a
   wrong definition, so a dump of any database that has a range type is not
   loadable. This is independent of whether pgschema manages the type.
3. **Cross-schema range columns lose their schema.** `rother.textrange`
   renders as `textrange`. Built-in `int4range` / `int4multirange` are fine.

## Catalog facts (verified on PostgreSQL 18, local; all apply to 14+)

| Catalog | Observation |
|---|---|
| `pg_type` | A range type is `typtype = 'r'`. Creating it also creates a multirange (`typtype = 'm'`) and two array types (`_timerange`, `_timemultirange`, `typtype = 'b'`, `typcategory = 'A'`). The multirange and the array have `pg_depend` rows with `deptype = 'i'` on the range, so `DROP TYPE range` removes all three. `typcollation` on the subtype row gives the subtype's default collation. |
| `pg_range` | One row per range: `rngtypid`, `rngsubtype`, `rngmultitypid` (14+), `rngcollation` (0 unless the subtype is collatable), `rngsubopc` (always set; the default opclass when none was given), `rngcanonical` and `rngsubdiff` (`regproc`, `0` / `-` when unset). |
| `pg_opclass` | `opcdefault` says whether `rngsubopc` is the subtype's default btree opclass. pg_dump prints `subtype_opclass` only when it is not. |
| `pg_proc` | The five constructors per range are `prolang = internal`, `prosrc` in `range_constructor2/3`, `multirange_constructor0/1/2`, and each has a `pg_depend` row `deptype = 'i'` on the type. pg_dump's `getFuncs` excludes any `pg_proc` row with an internal dependency: `NOT EXISTS (SELECT 1 FROM pg_depend WHERE classid = 'pg_proc'::regclass AND objid = p.oid AND deptype = 'i')`. |
| `information_schema.columns` | A range column is `data_type = 'USER-DEFINED'` with `udt_schema` / `udt_name`. The column query's `CASE` on `dt.typtype` has branches for `'d'`, `'e'`, `'c'`, `'b'` and falls through to `ELSE c.udt_name` for `'r'` and `'m'`, which drops the schema. |
| Multirange default name | `makeMultirangeTypeName` (`src/backend/catalog/pg_type.c`): if the range name contains `range`, replace the first occurrence with `multirange` (`timerange` → `timemultirange`, `my_range_t` → `my_multirange_t`); otherwise append `_multirange` (`floatspan` → `floatspan_multirange`). Clipped to 63 bytes. The multirange lives in the range's schema unless `multirange_type_name` was schema-qualified. |

Rules PostgreSQL enforces that shape the diff:

- There is no `ALTER TYPE` for a range's definition. Only `RENAME`, `OWNER`,
  `SET SCHEMA`. A changed subtype, opclass, collation, canonical, subtype_diff
  or multirange name means drop and recreate.
- `DROP TYPE range RESTRICT` fails while a column, domain, composite attribute
  or function signature uses the range or its multirange.
- `subtype` must have a btree opclass. Enums and domains qualify, so a range
  over a user-defined enum or domain in the same schema is legal and creates a
  type-to-type dependency.
- `subtype_diff` takes two subtype arguments and returns `float8`; it can be
  written in SQL or PL/pgSQL and does not reference the range type, so it only
  needs to exist before the `CREATE TYPE`.
- `canonical` takes and returns the range type itself, so it can only be
  created against a shell type: `CREATE TYPE r;` then the function, then
  `CREATE TYPE r AS RANGE (...)`. A shell type is a pseudo-type
  (`typtype = 'p'`, `typisdefined = false`), and only C and `internal`
  functions accept one. Verified: a SQL function fails with "SQL function
  cannot accept shell type", and a PL/pgSQL function fails with "PL/pgSQL
  functions cannot return type" even with `check_function_bodies = off`,
  because the PL/pgSQL validator rejects pseudo-type arguments and return
  types before it looks at that setting. So every canonical function is a C
  function backed by a shared library, which in practice means an extension.
  pg_dump registers a shell type for every range and prints it only when the
  canonical function needs it; pgschema does not need that bootstrap.

pg_dump's rendering (reference for the canonical format), options in this
order, each printed only when applicable:

```sql
CREATE TYPE rprobe.timerange AS RANGE (
    subtype = time without time zone,
    multirange_type_name = rprobe.timemultirange,   -- pg_dump always prints it on 14+
    subtype_opclass = pg_catalog.text_pattern_ops,  -- only when not the default opclass
    collation = pg_catalog."C",                     -- only when it differs from the subtype's
    canonical = rprobe.my_canonical,                -- only when set
    subtype_diff = rprobe.time_diff                 -- only when set
);
```

## Design

> Implementation note (same day): Phase 1 landed as planned. The internal
> dependency filter went on every `pg_proc` extension-member filter in
> `queries.sql` (functions, procedures, aggregates, privileges), and the
> `testdata/dump/range/` suite was added with a `pgschema.sql` that still
> lacks the `CREATE TYPE ... AS RANGE` statements; Phase 2 regenerates it.
>
> Phases 2 and 3 landed together after that. Two deviations from the text
> below: `COMMENT ON TYPE` is not generated for any type kind today (the IR
> captures `Type.Comment`, but no diff code emits it), so range comments are
> not round-tripped either and the `add_range_type` case carries no comment;
> and the `subtype_diff` include case was dropped because `main.sql` includes
> the whole `types/` folder before any function, so it cannot express that
> ordering. The `add_range_subtype_diff` diff case covers it through apply.
> One addition: functions are dropped before types, so dropping a range and
> its `subtype_diff` function together failed with a dependency error; the
> drop phase now holds back a dropped function that a dropped type (range or
> domain) still references until after the types are gone
> (`drop_range_with_subtype_diff`).
>
> Review of PR #684 moved a few things from the text below:
> - A modified range is not replaced in the enum-ADD VALUE slot. The drop
>   phase comes first in a plan, so the replacement (still one DROP + CREATE
>   step) is scheduled in the create phase through the same buckets a new
>   range uses: after the tables that used the old definition are dropped,
>   and after a new `subtype_diff` function or subtype it needs
>   (`alter_range_with_dropped_table`, `alter_range_add_subtype_diff`).
> - Deferral propagates: a composite with a range attribute, a domain over
>   its multirange, a table with an array column of either, and a function
>   whose signature uses the range all wait for a deferred range
>   (`add_range_subtype_diff`).
> - Types are dropped in reverse dependency order, and a held-back
>   `subtype_diff` function is dropped between the types that reference it
>   and the types its signature uses, with the functions it calls held too
>   (`drop_range_subtype_diff_chain`). That last part exposed an inspector
>   bug: `GetFunctionDependencies` keyed functions by identity arguments
>   (with parameter names) while the Functions map is keyed by types, so
>   named-parameter functions never had dependencies. It now uses
>   `oidvectortypes`.
> - `makeMultirangeTypeName` clips the range name to NAMEDATALEN-12 bytes
>   before appending `_multirange`; the port does the same.
> - Grants on a replaced range are lost, like grants on a recreated function
>   or view today (noted under #677); not addressed here.


### IR

```go
const TypeKindRange TypeKind = "RANGE"

type Type struct {
    // ...
    // Range types (Kind == TypeKindRange). Subtype follows the domain
    // BaseType convention: format_type for pg_catalog types, otherwise
    // quote_ident-qualified, with the target-schema prefix stripped at
    // render time. Function references follow the aggregate convention:
    // bare when in the type's own schema, qualified otherwise, emitted
    // verbatim.
    Subtype            string `json:"subtype,omitempty"`
    SubtypeOpclass     string `json:"subtype_opclass,omitempty"`     // empty when it is the subtype's default
    Collation          string `json:"collation,omitempty"`           // empty when it equals the subtype's collation
    Canonical          string `json:"canonical,omitempty"`
    SubtypeDiff        string `json:"subtype_diff,omitempty"`
    MultirangeTypeName string `json:"multirange_type_name,omitempty"` // always populated by the inspector
}
```

`MultirangeTypeName` is always captured so `typesEqual` can compare it
directly (both sides come from inspection). The renderer decides whether to
print it: only when it differs from the PostgreSQL default computed by a Go
port of `makeMultirangeTypeName`, or when it lives in another schema. This
keeps the dump minimal and identical to what a user would write, while a
renamed multirange is still detected as a change.

`SubtypeOpclass` and `Collation` are stored only when non-default, matching
pg_dump's print rule, so "default" compares equal across versions and the
golden files stay byte-stable.

### Inspector

- New query `GetRangeTypesForSchema`, shaped like `GetDomainsForSchema`:
  `pg_range` joined to `pg_type` (the range, the subtype, the multirange),
  `pg_namespace`, `pg_opclass` with its namespace, `pg_collation` with its
  namespace, `pg_proc` twice (canonical, subtype_diff) with their namespaces,
  `pg_description`. Filter `t.typtype = 'r'`, target schema, and the usual
  extension-member `NOT EXISTS`. The subtype column reuses the `GetDomains`
  `base_type` `CASE` verbatim (pg_catalog → `format_type`, array → element
  qualification, otherwise `quote_ident(schema).quote_ident(name)`).
  Collation follows the column-collation rule from issue #593:
  `quote_ident(collname)` for pg_catalog, qualified otherwise, empty when
  `rngcollation = 0 OR rngcollation = st.typcollation`. Opclass is
  `quote_ident(nsp).quote_ident(opcname)` unless `opc.opcdefault`.
  Functions: `quote_ident(proname)` when `pn.nspname = n.nspname`, qualified
  otherwise, empty when the oid is 0.
- `buildTypes` gains a third loop after domains that sets `Kind =
  TypeKindRange`, applies `ShouldIgnoreType`, and calls `SetType`. The
  existing `GetTypesForSchema` keeps `typtype IN ('e', 'c')`, so multiranges
  (`'m'`) and the auto array types never show up as types of their own.
- Column type queries (both `CASE ... dt.typtype` blocks in
  `ir/queries/queries.sql`): add a branch before the `ELSE`:

  ```sql
  WHEN dt.typtype IN ('r', 'm') THEN
      CASE WHEN dn.nspname = 'pg_catalog' THEN dt.typname::text
           ELSE quote_ident(dn.nspname) || '.' || quote_ident(dt.typname) END
  ```

  Arrays of ranges already go through the `typcategory = 'A'` branch, which
  qualifies the element the same way. Composite attribute types, domain base
  types and function signatures use generic qualification and need no change.
- Function queries (`GetFunctions`, `GetFunctionsForSchema` and the
  procedure variants): add pg_dump's
  `NOT EXISTS (... pg_depend ... classid = 'pg_proc' ... deptype = 'i')`.
  This removes the five constructors per range type. Procedures cannot have
  internal dependencies, but the filter is harmless and keeps the queries
  parallel. The function-privilege queries already require
  `p.proacl IS NOT NULL`, which the constructors never satisfy unless someone
  grants on them; add the same filter there for consistency.
- Type-privilege query (`typtype IN ('e', 'c', 'd')`): add `'r'`, so
  `GRANT USAGE ON TYPE timerange` round-trips. Grants on the multirange are
  out of scope (see below).
- `cmd/plan/plan.go` schema rename already rewrites `typ.Schema`; because
  same-schema references are stored bare, nothing else needs rewriting.
- `ir/normalize.go`: `normalizeType` stays domain-only. `Subtype` comes from
  `format_type` and needs no normalization, like a domain's `BaseType`.

### Dump and CREATE TYPE

`generateTypeSQL` gets a `TypeKindRange` case. Multi-line, 4-space indent to
match the enum format, options in pg_dump's order, each on its own line:

```sql
CREATE TYPE timerange AS RANGE (
    subtype = time without time zone,
    subtype_diff = time_diff
);

CREATE TYPE textrange AS RANGE (
    subtype = text,
    subtype_opclass = pg_catalog.text_pattern_ops,
    collation = "C"
);

CREATE TYPE floatspan AS RANGE (
    subtype = double precision,
    multirange_type_name = floatspans
);
```

- `subtype` goes through `stripSchemaPrefixMode` like a domain's base type.
- `multirange_type_name` is printed when it differs from the Go
  `makeMultirangeTypeName` port or has a schema prefix after stripping.
- `subtype_opclass` is always schema-qualified (pg_dump does the same; the
  opclass namespace is never the target schema in practice).
- `collation`, `canonical`, `subtype_diff` are emitted verbatim from the IR.
- `DROP TYPE IF EXISTS timerange RESTRICT;` through the existing path. The
  multirange and both array types go with it.
- `DiffType` stays `DiffTypeType` ("type"), so `plan.txt`, `plan.json`, the
  multi-file dump (`types/timerange.sql`) and the comment path need no new
  cases. `COMMENT ON TYPE` works through `Type.Comment` unchanged.

### Ordering

- `topologicallySortTypes`: a range gets an edge from the type named by
  `Subtype` when it is in the set (enum or domain subtype), via the existing
  `extractTypeName`. Composite attributes and domain base types that name a
  range are already covered by the existing edges.
- `collectMigrationSQL` type buckets (`diff.go` around `typesWithoutDeps`):
  a range whose `SubtypeDiff` names a function in
  `newFunctionLookup` joins `domainsWithFunctionDepsOnly` (rename to
  `typesWithFunctionDepsOnly`) and its name goes into `functionDomainLookup`,
  so tables with a column of that type (or of its multirange: add both names
  to the lookup) fall into table batch 1, after functions and deferred types.
  Everything else goes into `typesWithoutDeps`, before sequences and tables.
- Multi-file dump: the formatter orders files by first appearance in the
  diff, so a deferred range's `types/timerange.sql` is included after
  `functions/time_diff.sql`, exactly as a function-dependent domain is today.
  Verify with an include test.
- `Canonical` is captured and rendered verbatim but never ordered against
  anything: the function has to be C (see catalog facts), so pgschema cannot
  create it and the type can only be created where it already exists, which
  is an extension-provided function. No shell type is emitted.

### Modified range types

`typesEqual` gets a `TypeKindRange` case comparing all six fields. Because
PostgreSQL offers no `ALTER`, a changed range is replaced: a new
`generateReplaceRangeTypesSQL` emits `DROP TYPE ... RESTRICT;` followed by the
`CREATE TYPE`, collected as `DiffOperationAlter` in the slot right after
`generateModifyEnumsSQL` (before the create phase, so a new table can use the
new definition). When a column still uses the type the `DROP` fails with
PostgreSQL's own dependency error, which is the honest outcome; the user
changes or drops the column in the same file to proceed.

The alternative is the composite precedent: a changed composite type is
silently reported as "No changes detected" today. That hides divergence from
the declared state and is not worth copying.

### Plan command

A column whose range type lives in another schema (`rother.textrange`) is
handled like a cross-schema enum column today: the plan database must already
have that schema, or the file must create it. No stub work for types in this
design; the fix here is only that the column type is qualified correctly once
inspected.

### Docs

- `docs/syntax/create_type.mdx`: add `CREATE TYPE type_name AS RANGE (
  subtype = ... )` to the syntax block with the five optional attributes, a
  bullet list of supported features, the canonical format above, and a note
  that a changed range definition is dropped and recreated.
- `README.md` Types row and `docs/coding-agent.mdx`: add "range".
- `CLAUDE.md` Supported Schema Objects, Types line: add "range".
- `docs/blog/supabase-declarative-schema-vs-pgschema.mdx`: flip the Range
  types row.

## Tests

Diff cases under `testdata/diff/create_type/` (each with `old.sql`,
`new.sql`, generated `diff.sql` / `plan.*`):

| Case | Checks |
|---|---|
| `add_range_type` | From empty: `CREATE TYPE floatrange AS RANGE (subtype = double precision)` printed before a table with a `floatrange` column, a `floatmultirange` column and `EXCLUDE USING gist (r WITH &&)`; `COMMENT ON TYPE`; no constructor functions in the plan. Added to `essentialTests` so 14-17 cover it. |
| `add_range_options` | `text` subtype with `collation = "C"`, `subtype_opclass = pg_catalog.text_pattern_ops` and `multirange_type_name = textspans`; all three rendered; a second range with the default multirange name prints no `multirange_type_name`. |
| `add_range_subtype_diff` | Function, range and table in one file: function first, range after the functions phase, table after the range. |
| `add_range_enum_subtype` | Range over a new enum: enum before range. |
| `add_range_in_composite_and_domain` | A composite with a range attribute and a domain over a range: range first. |
| `drop_range_type` | Table and range removed: `DROP TABLE` then `DROP TYPE ... RESTRICT`. |
| `alter_range_subtype` | Subtype changes with no dependents: `DROP TYPE` then `CREATE TYPE` in the alter slot. |
| `range_column_unchanged` | Both sides have the range and a column using it: no changes detected. |

`privilege/` gets `grant_type_range` (`GRANT USAGE ON TYPE`), and
`comment/` gets a range case only if the comment path turns out to need one.

A `testdata/dump/range/` suite (`manifest.json`, `raw.sql`, `pgdump.sql`,
`pgschema.sql`) built from the probe schema above, registered with
`runExactMatchTest` in `cmd/dump/dump_integration_test.go`. `pgdump.sql` is
taken from pg_dump 18 with the `\restrict` lines removed and
`SET transaction_timeout` commented out, per the version gotcha in
`CLAUDE.md`. A `testdata/include/types/price_range.sql` case covers the
multi-file layout, and a second include case with a `subtype_diff` function
checks that the type file is included after the function file.

Regression: `testdata/dump/issue_125_function_default` already has an
`int4range` column and must stay byte-identical. Run the full matrix with
`PGSCHEMA_POSTGRES_VERSION` 14 through 18; nothing here is version-gated
since multiranges exist on 14.

## Phases

**Phase 1 — stop the bleeding.** The two inspector fixes that apply to any
database with a range type, managed or not: exclude internally-dependent
`pg_proc` rows from the function queries, and qualify `'r'` / `'m'` column
types. Small, self-contained, shippable as its own PR with the dump suite's
`raw.sql` as the regression. After this, a dump of a database with range
types loads once the user adds the `CREATE TYPE` by hand.

**Phase 2 — inspect, dump, create, drop.** `TypeKindRange`, the
`GetRangeTypesForSchema` query, `buildTypes` loop, `generateTypeSQL` case,
`typesEqual`, the multirange-name port, topological edge, type privileges,
docs, and the first six diff cases plus the dump suite. Comparable to the
domain support minus `ALTER DOMAIN`.

**Phase 3 — functions and replacement.** Deferred bucket for
`subtype_diff`, the replace path for modified ranges, the remaining diff
cases and include tests. About half of Phase 2.

Phase 2 ships on its own: until Phase 3, a range whose `subtype_diff` names
a function created in the same plan is emitted before that function and
fails at apply, and a modified range is treated like a modified composite
(no DDL). Both are strictly better than today's silent omission.

## Out of scope

- Comments and privileges on the auto-created multirange type. They are
  separate objects in the catalog; pgschema treats them as part of the range.
- Stubbing cross-schema range types in the plan database. Same limitation as
  cross-schema enums and composites today.
- `ALTER TYPE ... RENAME` / `SET SCHEMA`. pgschema has no rename detection
  for any object.
- Creating canonical functions. They can only be written in C (the shell
  type they take is a pseudo-type that SQL and PL/pgSQL reject), so they need
  a shared library on the server and belong to extensions, which pgschema
  already excludes. `canonical = ...` is still dumped and compared when the
  function exists on the target.
