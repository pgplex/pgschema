--
-- PostgreSQL database dump
--

SET statement_timeout = 0;
SET lock_timeout = 0;
-- SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgschema_repro_nulls; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pgschema_repro_nulls (
    a integer,
    b integer
);

--
-- Name: pgschema_repro_nulls pgschema_repro_nulls_uniq; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pgschema_repro_nulls
    ADD CONSTRAINT pgschema_repro_nulls_uniq UNIQUE NULLS NOT DISTINCT (a, b);

--
-- PostgreSQL database dump complete
--
