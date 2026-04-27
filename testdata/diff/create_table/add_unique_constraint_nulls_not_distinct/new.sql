-- Regression for: UNIQUE NULLS NOT DISTINCT modifier dropped from constraints
-- Add a table-level UNIQUE constraint with the NULLS NOT DISTINCT modifier
-- (PostgreSQL 15+). Without the fix, the inspector loses the modifier and the
-- generated migration emits a plain UNIQUE (a, b) — silently changing the
-- semantics of the constraint and breaking INSERT ... ON CONFLICT flows that
-- rely on NULLs colliding.
CREATE TABLE public.pgschema_repro_nulls (
    a integer,
    b integer,
    CONSTRAINT pgschema_repro_nulls_uniq UNIQUE NULLS NOT DISTINCT (a, b)
);
