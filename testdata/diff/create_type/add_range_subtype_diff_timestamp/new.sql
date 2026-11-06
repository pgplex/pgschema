-- The function's parameter types are normalized to the short spelling
-- (timestamp) while the range's subtype keeps the format_type spelling
-- (timestamp without time zone); the two must still match, so the function
-- is created before the range and the table after it.
CREATE FUNCTION ts_diff(x timestamp, y timestamp) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE stamp_span AS RANGE (subtype = timestamp, subtype_diff = ts_diff);

CREATE TABLE stamps (
    id integer PRIMARY KEY,
    during stamp_span
);
