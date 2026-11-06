-- The new definition names a function created in the same plan, so the
-- replacement waits for the functions phase like a new range would.
CREATE FUNCTION int_diff(x integer, y integer) RETURNS double precision
    AS 'SELECT (x - y)::double precision' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE span AS RANGE (subtype = integer, subtype_diff = int_diff);
