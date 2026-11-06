CREATE TYPE severity AS ENUM ('low', 'high');

CREATE FUNCTION severity_num(s severity) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN CASE s WHEN 'low' THEN 0 ELSE 1 END;

CREATE FUNCTION severity_diff(a severity, b severity) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    RETURN severity_num(a) - severity_num(b);

CREATE TYPE band AS RANGE (subtype = severity, subtype_diff = severity_diff);

CREATE DOMAIN band_set AS band_multirange;
