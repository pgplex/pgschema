CREATE FUNCTION int_diff(x integer, y integer) RETURNS double precision
    AS 'SELECT (x - y)::double precision' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE span AS RANGE (subtype = integer, subtype_diff = int_diff);
