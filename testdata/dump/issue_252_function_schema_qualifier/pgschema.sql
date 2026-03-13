--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.7.0


--
-- Name: users; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS users (
    id SERIAL,
    name text NOT NULL,
    email text,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

--
-- Name: get_table_info(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_table_info()
RETURNS text
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN 'Table: public.users';
END;
$$;

--
-- Name: get_user_by_name(text); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_user_by_name(
    p_name text
)
RETURNS TABLE(id integer, name text, email text)
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN QUERY SELECT u.id, u.name, u.email FROM public.users u WHERE u.name = p_name;
END;
$$;

--
-- Name: get_user_count(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION get_user_count()
RETURNS integer
LANGUAGE plpgsql
VOLATILE
AS $$
BEGIN
    RETURN (SELECT count(*)::integer FROM public.users);
END;
$$;

--
-- Name: insert_user(text, text); Type: PROCEDURE; Schema: -; Owner: -
--

CREATE OR REPLACE PROCEDURE insert_user(
    IN p_name text,
    IN p_email text
)
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO public.users (name, email) VALUES (p_name, p_email);
END;
$$;

