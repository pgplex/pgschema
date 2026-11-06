CREATE FUNCTION is_positive(v integer) RETURNS boolean
    AS 'SELECT v > 0' LANGUAGE sql STRICT IMMUTABLE;

CREATE DOMAIN positive AS integer CHECK (is_positive(VALUE));

CREATE FUNCTION positive_diff(a positive, b positive) RETURNS double precision
    AS 'SELECT (a - b)::double precision' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE positive_span AS RANGE (subtype = positive, subtype_diff = positive_diff);
