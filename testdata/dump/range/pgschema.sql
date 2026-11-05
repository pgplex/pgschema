--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.1
-- Dumped by pgschema version 1.13.4


--
-- Name: reservations; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS reservations (
    id integer,
    during timerange,
    slots floatspan,
    free floatspans,
    label other.textrange,
    seats int4range,
    rooms int4multirange,
    CONSTRAINT reservations_pkey PRIMARY KEY (id),
    CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&)
);

--
-- Name: time_diff(time without time zone, time without time zone); Type: FUNCTION; Schema: -; Owner: -
--

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

