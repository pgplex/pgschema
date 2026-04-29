--
-- Test case for GitHub issue #412: UNIQUE NULLS NOT DISTINCT dropped from dump
--
-- The NULLS NOT DISTINCT modifier (PostgreSQL 15+) makes NULL-bearing tuples
-- collide for uniqueness purposes, which is the opposite of the SQL default.
-- pgschema dump used to silently drop the modifier, emitting a plain
-- UNIQUE (...) constraint and quietly changing semantics.
--

CREATE TABLE pgschema_repro_nulls (
    a integer,
    b integer,
    CONSTRAINT pgschema_repro_nulls_uniq UNIQUE NULLS NOT DISTINCT (a, b)
);
