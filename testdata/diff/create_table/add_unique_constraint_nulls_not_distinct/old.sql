-- Regression for: UNIQUE NULLS NOT DISTINCT modifier dropped from constraints
-- The starting state has no UNIQUE constraint at all; we add one in new.sql.
CREATE TABLE public.pgschema_repro_nulls (
    a integer,
    b integer
);
