--
-- PostgreSQL database dump
--

SET statement_timeout = 0;
SET lock_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: test_table; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.test_table (
    id integer NOT NULL,
    status text NOT NULL,
    reason text,
    actor_id uuid
);

--
-- Name: test_table test_table_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.test_table
    ADD CONSTRAINT test_table_pkey PRIMARY KEY (id);

--
-- Name: test_table test_table_status_check; Type: CHECK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.test_table
    ADD CONSTRAINT test_table_status_check CHECK (((status = 'active'::text) OR ((status = 'cancelled'::text) AND (reason IS NOT NULL)) OR ((status = 'revoked'::text) AND (actor_id IS NOT NULL))));

--
-- PostgreSQL database dump complete
--
