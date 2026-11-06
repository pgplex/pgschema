CREATE OR REPLACE FUNCTION int_diff(
    x integer,
    y integer
)
RETURNS double precision
LANGUAGE sql
IMMUTABLE
STRICT
AS $$SELECT (x - y)::double precision
$$;

DROP TYPE IF EXISTS span RESTRICT;

CREATE TYPE span AS RANGE (
    subtype = integer,
    subtype_diff = int_diff
);
