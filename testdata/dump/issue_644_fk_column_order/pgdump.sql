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

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: c1; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.c1 (
    a integer,
    b integer
);


--
-- Name: c2; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.c2 (
    a integer,
    b integer
);


--
-- Name: p; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.p (
    a integer NOT NULL,
    b integer NOT NULL
);


--
-- Name: p p_b_a_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.p
    ADD CONSTRAINT p_b_a_key UNIQUE (b, a);


--
-- Name: p p_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.p
    ADD CONSTRAINT p_pkey PRIMARY KEY (a, b);


--
-- Name: c1 fk_p; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.c1
    ADD CONSTRAINT fk_p FOREIGN KEY (a, b) REFERENCES public.p(a, b);


--
-- Name: c2 fk_p; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.c2
    ADD CONSTRAINT fk_p FOREIGN KEY (b, a) REFERENCES public.p(b, a);


--
-- PostgreSQL database dump complete
--


