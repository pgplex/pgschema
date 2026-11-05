--
-- PostgreSQL database dump
--


-- Dumped from database version 18.1 (Postgres.app)
-- Dumped by pg_dump version 18.1 (Postgres.app)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
-- SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: other; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA other;


--
-- Name: textrange; Type: TYPE; Schema: other; Owner: -
--

CREATE TYPE other.textrange AS RANGE (
    subtype = text,
    multirange_type_name = other.textmultirange,
    subtype_opclass = pg_catalog.text_pattern_ops,
    collation = pg_catalog."C"
);


--
-- Name: floatspan; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.floatspan AS RANGE (
    subtype = double precision,
    multirange_type_name = public.floatspans
);


--
-- Name: time_diff(time without time zone, time without time zone); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.time_diff(x time without time zone, y time without time zone) RETURNS double precision
    LANGUAGE sql IMMUTABLE STRICT
    AS $$SELECT EXTRACT(EPOCH FROM (x - y))$$;


--
-- Name: timerange; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.timerange AS RANGE (
    subtype = time without time zone,
    multirange_type_name = public.timemultirange,
    subtype_diff = public.time_diff
);


--
-- Name: TYPE timerange; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TYPE public.timerange IS 'Range of times of day';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: reservations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reservations (
    id integer NOT NULL,
    during public.timerange,
    slots public.floatspan,
    free public.floatspans,
    label other.textrange,
    seats int4range,
    rooms int4multirange
);


--
-- Name: reservations reservations_during_excl; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_during_excl EXCLUDE USING gist (during WITH &&);


--
-- Name: reservations reservations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reservations
    ADD CONSTRAINT reservations_pkey PRIMARY KEY (id);


--
-- PostgreSQL database dump complete
--


