CREATE TYPE z_status AS ENUM ('span', 'off');

-- The subtype_diff body mentions the range's own name inside a string
-- literal and also casts to the enum. The literal must not be mistaken for
-- a dependency on the range, which would contradict the range's dependency
-- on this function; the enum cast is a real one.
CREATE FUNCTION span_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN 'span'::z_status = 'span' THEN (x - y)::double precision ELSE 0 END;

CREATE TYPE span AS RANGE (subtype = integer, subtype_diff = span_diff);
