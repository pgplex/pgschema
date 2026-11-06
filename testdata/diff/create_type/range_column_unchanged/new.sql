CREATE FUNCTION time_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE timerange AS RANGE (subtype = time, subtype_diff = time_diff);

CREATE TYPE textrange AS RANGE (
    subtype = text,
    subtype_opclass = text_pattern_ops,
    collation = "C",
    multirange_type_name = textspans
);

CREATE TABLE shifts (
    id integer PRIMARY KEY,
    during timerange,
    breaks timemultirange,
    label textrange,
    labels textspans
);
