CREATE OR REPLACE FUNCTION ts_diff(
    x timestamp,
    y timestamp
)
RETURNS double precision
LANGUAGE sql
IMMUTABLE
STRICT
AS $$SELECT EXTRACT(EPOCH FROM (x - y))
$$;

CREATE TYPE stamp_span AS RANGE (
    subtype = timestamp without time zone,
    subtype_diff = ts_diff
);

CREATE TABLE IF NOT EXISTS stamps (
    id integer,
    during stamp_span,
    CONSTRAINT stamps_pkey PRIMARY KEY (id)
);
