--
-- PostgreSQL database dump
--

-- Dumped from database version 17.5 (Debian 17.5-1.pgdg120+1)
-- Dumped by pg_dump version 17.6 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: test_func(integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.test_func(a integer) RETURNS integer
    LANGUAGE plpgsql
    AS $$
BEGIN
    RETURN a * 2;
END;
$$;


--
-- Name: test_func(text); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.test_func(a text) RETURNS text
    LANGUAGE plpgsql
    AS $$
BEGIN
    RETURN 'Hello, ' || a;
END;
$$;


--
-- Name: test_func(integer, integer); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.test_func(a integer, b integer) RETURNS integer
    LANGUAGE plpgsql
    AS $$
BEGIN
    RETURN a + b;
END;
$$;


--
-- Name: test_proc(integer); Type: PROCEDURE; Schema: public; Owner: -
--

CREATE PROCEDURE public.test_proc(IN a integer)
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE NOTICE 'Integer: %', a;
END;
$$;


--
-- Name: test_proc(text); Type: PROCEDURE; Schema: public; Owner: -
--

CREATE PROCEDURE public.test_proc(IN a text)
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE NOTICE 'Text: %', a;
END;
$$;


--
-- Name: test_proc(integer, integer); Type: PROCEDURE; Schema: public; Owner: -
--

CREATE PROCEDURE public.test_proc(IN a integer, IN b integer)
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE NOTICE 'Sum: %', a + b;
END;
$$;


--
-- Name: provide_tx(text[]); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.provide_tx(VARIADIC p_txs text[])
    RETURNS void
    LANGUAGE sql
    AS $$
SELECT 1;
$$;


--
-- Name: provide_tx(uuid); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.provide_tx(p_id uuid)
    RETURNS void
    LANGUAGE plpgsql
    AS $$
BEGIN
    RAISE NOTICE '%', p_id;
END;
$$;


--
-- PostgreSQL database dump complete
--
