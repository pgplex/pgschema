-- The range names a function created in the same plan as its subtype_diff, so
-- it is created after the functions phase. Everything built on it waits with
-- it: a composite with a range attribute, a domain over the multirange, a
-- table with array columns of both, and a function taking the range.
CREATE FUNCTION time_diff(x time, y time) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (x - y))' LANGUAGE sql STRICT IMMUTABLE;

CREATE TYPE timerange AS RANGE (subtype = time, subtype_diff = time_diff);

CREATE TYPE shift_slot AS (room integer, during timerange);

CREATE DOMAIN break_set AS timemultirange;

CREATE TABLE shifts (
    id integer PRIMARY KEY,
    during timerange,
    breaks timemultirange,
    history timerange[],
    past_breaks timemultirange[]
);

CREATE FUNCTION shift_length(r timerange) RETURNS double precision
    AS 'SELECT EXTRACT(EPOCH FROM (upper(r) - lower(r)))' LANGUAGE sql STRICT IMMUTABLE;
