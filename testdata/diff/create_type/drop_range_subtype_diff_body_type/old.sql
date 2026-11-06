CREATE TYPE z_status AS ENUM ('on', 'off');

-- A SQL-standard body is parsed at creation, so PostgreSQL records the
-- function's dependency on the enum it casts to.
CREATE FUNCTION span_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN 'on'::z_status = 'on' THEN (x - y)::double precision ELSE 0 END;

CREATE TYPE span AS RANGE (subtype = integer, subtype_diff = span_diff);
