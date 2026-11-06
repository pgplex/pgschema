CREATE OR REPLACE FUNCTION time_diff(
    x time without time zone,
    y time without time zone
)
RETURNS double precision
LANGUAGE sql
IMMUTABLE
STRICT
AS $$SELECT EXTRACT(EPOCH FROM (x - y))
$$;

CREATE TYPE timerange AS RANGE (
    subtype = time without time zone,
    subtype_diff = time_diff
);

CREATE DOMAIN break_set AS timemultirange;

CREATE TYPE shift_slot AS (room integer, during timerange);

CREATE TABLE IF NOT EXISTS shifts (
    id integer,
    during timerange,
    breaks timemultirange,
    history timerange[],
    past_breaks timemultirange[],
    CONSTRAINT shifts_pkey PRIMARY KEY (id)
);

CREATE OR REPLACE FUNCTION shift_length(
    r timerange
)
RETURNS double precision
LANGUAGE sql
IMMUTABLE
STRICT
AS $$SELECT EXTRACT(EPOCH FROM (upper(r) - lower(r)))
$$;
