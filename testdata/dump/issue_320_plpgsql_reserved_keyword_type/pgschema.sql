--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.7.2


--
-- Name: user; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS "user" (
    id SERIAL,
    name text NOT NULL,
    email text,
    CONSTRAINT user_pkey PRIMARY KEY (id)
);

--
-- Name: count_users(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION count_users()
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    total_count integer;
BEGIN
    SELECT count(*)::integer INTO total_count FROM public."user";
    RETURN total_count;
END;
$$;

--
-- Name: get_first_user(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_first_user()
RETURNS text
LANGUAGE plpgsql
VOLATILE
AS $$
DECLARE
    account public.user;
BEGIN
    SELECT * INTO account FROM public.user LIMIT 1;
    RETURN account.name;
END;
$$;

