CREATE FUNCTION a_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN 'z_span' = 'z_span' THEN (x - y)::double precision ELSE 0 END;

CREATE TYPE a_span AS RANGE (subtype = integer, subtype_diff = a_diff);

-- z_diff's body casts to a_span, so PostgreSQL records that z_diff depends on
-- a_span; a_diff's body only mentions z_span in a string literal, which is
-- not a dependency.
CREATE FUNCTION z_diff(x integer, y integer) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE WHEN '[1,2)'::a_span @> 1 THEN (x - y)::double precision ELSE 0 END;

CREATE TYPE z_span AS RANGE (subtype = integer, subtype_diff = z_diff);
