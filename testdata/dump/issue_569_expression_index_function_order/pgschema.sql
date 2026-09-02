--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.12.5


--
-- Name: normalize_name(text); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION normalize_name(
    text
)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
AS $_$
BEGIN
    RETURN lower(trim($1));
END;
$_$;

--
-- Name: users; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS users (
    id SERIAL,
    full_name text NOT NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

--
-- Name: idx_users_normalized; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS idx_users_normalized ON users (normalize_name(full_name));

