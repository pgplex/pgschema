CREATE FUNCTION span_diff(x integer, y integer) RETURNS double precision
    AS 'SELECT (x - y)::double precision' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE span AS RANGE (subtype = integer, subtype_diff = span_diff);

-- An overload of the subtype_diff name that takes the range itself: it
-- depends on the range and is not its subtype_diff.
CREATE FUNCTION span_diff(r span) RETURNS double precision
    AS 'SELECT (upper(r) - lower(r))::double precision' LANGUAGE sql STRICT IMMUTABLE;
