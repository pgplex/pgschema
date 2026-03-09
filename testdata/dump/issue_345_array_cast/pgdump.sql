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
-- Name: repro; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.repro (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    data jsonb DEFAULT '{}'::jsonb
);

--
-- Name: repro repro_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.repro
    ADD CONSTRAINT repro_pkey PRIMARY KEY (id);

--
-- Name: repro; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.repro ENABLE ROW LEVEL SECURITY;

--
-- Name: repro p; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY p ON public.repro USING (((data #>> ('{nested,key}'::text[])) = 'x'::text));

--
-- PostgreSQL database dump complete
--
