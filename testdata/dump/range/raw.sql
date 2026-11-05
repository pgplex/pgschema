--
-- User-defined range types: the constructors PostgreSQL creates for them must
-- not be dumped as functions, and range-typed columns must keep their schema
--

CREATE SCHEMA other;

CREATE FUNCTION time_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;

-- subtype_diff and a comment
CREATE TYPE timerange AS RANGE (subtype = time, subtype_diff = time_diff);
COMMENT ON TYPE timerange IS 'Range of times of day';

-- explicit multirange name
CREATE TYPE floatspan AS RANGE (subtype = float8, multirange_type_name = floatspans);

-- non-default collation and operator class, in another schema
CREATE TYPE other.textrange AS RANGE (
    subtype = text,
    subtype_opclass = text_pattern_ops,
    collation = "C"
);

CREATE TABLE reservations (
    id integer PRIMARY KEY,
    during timerange,
    slots floatspan,
    free floatspans,
    label other.textrange,
    seats int4range,
    rooms int4multirange,
    CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&)
);
