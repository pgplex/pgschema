--
-- PostgreSQL database dump
--

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: vt; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vt (
    slug text NOT NULL,
    identifier text GENERATED ALWAYS AS (('urn:sdp:catalog:'::text || slug)) VIRTUAL
);

--
-- PostgreSQL database dump complete
--

