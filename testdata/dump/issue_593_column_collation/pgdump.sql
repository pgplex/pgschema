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
-- Name: my_coll; Type: COLLATION; Schema: public; Owner: -
--

CREATE COLLATION public.my_coll (provider = libc, locale = 'C');

--
-- Name: recipient_search; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.recipient_search (
    id bigint NOT NULL,
    search_text text COLLATE pg_catalog."C" NOT NULL,
    display_name character varying(100) COLLATE pg_catalog."POSIX" DEFAULT ''::character varying,
    note text,
    region text COLLATE public.my_coll
);

--
-- Name: recipient_search recipient_search_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.recipient_search
    ADD CONSTRAINT recipient_search_pkey PRIMARY KEY (id);

--
-- Name: recipient_search_prefix_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX recipient_search_prefix_idx ON public.recipient_search USING btree (search_text text_pattern_ops);

--
-- PostgreSQL database dump complete
--
