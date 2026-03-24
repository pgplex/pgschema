--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.7.4


--
-- Name: provide_tx(text[]); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION provide_tx(
    VARIADIC p_txs text[]
)
RETURNS void
LANGUAGE sql
VOLATILE
AS $$
SELECT 1;
$$;

--
-- Name: provide_tx(uuid); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION provide_tx(
    p_id uuid
)
RETURNS void
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RAISE NOTICE '%', p_id;
END;
$$;

--
-- Name: test_func(integer); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION test_func(
    a integer
)
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN a * 2;
END;
$$;

--
-- Name: test_func(integer, integer); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION test_func(
    a integer,
    b integer
)
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN a + b;
END;
$$;

--
-- Name: test_func(text); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION test_func(
    a text
)
RETURNS text
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN 'Hello, ' || a;
END;
$$;

--
-- Name: test_proc(integer); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE test_proc(
    IN a integer
)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Integer: %', a;
END;
$$;

--
-- Name: test_proc(integer, integer); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE test_proc(
    IN a integer,
    IN b integer
)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Sum: %', a + b;
END;
$$;

--
-- Name: test_proc(text); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE test_proc(
    IN a text
)
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE NOTICE 'Text: %', a;
END;
$$;

