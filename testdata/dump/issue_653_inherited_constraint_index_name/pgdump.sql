--
-- PostgreSQL database dump
--


-- Dumped from database version 17.6 (Homebrew)
-- Dumped by pg_dump version 17.6 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
-- SET transaction_timeout = 0;
SET client_encoding = 'SQL_ASCII';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: event_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event_notes (
    event_id integer NOT NULL,
    note text
);


--
-- Name: events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events (
    id integer NOT NULL,
    region text NOT NULL
)
PARTITION BY RANGE (id);


--
-- Name: events_archive; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events_archive (
    id integer NOT NULL,
    region text NOT NULL
)
PARTITION BY RANGE (id);


--
-- Name: events_archive_old; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events_archive_old (
    id integer NOT NULL,
    region text NOT NULL
);


--
-- Name: events_leaf; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events_leaf (
    id integer NOT NULL,
    region text NOT NULL
);


--
-- Name: events_archive; Type: TABLE ATTACH; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events ATTACH PARTITION public.events_archive FOR VALUES FROM (10) TO (20);


--
-- Name: events_archive_old; Type: TABLE ATTACH; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_archive ATTACH PARTITION public.events_archive_old FOR VALUES FROM (10) TO (15);


--
-- Name: events_leaf; Type: TABLE ATTACH; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events ATTACH PARTITION public.events_leaf FOR VALUES FROM (0) TO (10);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);


--
-- Name: events_archive events_archive_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_archive
    ADD CONSTRAINT events_archive_pkey PRIMARY KEY (id);


--
-- Name: events_archive_old events_archive_old_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_archive_old
    ADD CONSTRAINT events_archive_old_pk PRIMARY KEY (id);


--
-- Name: events events_region_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_region_id_key UNIQUE (region, id);


--
-- Name: events_archive events_archive_region_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_archive
    ADD CONSTRAINT events_archive_region_id_key UNIQUE (region, id);


--
-- Name: events_archive_old events_archive_old_region_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_archive_old
    ADD CONSTRAINT events_archive_old_region_id_key UNIQUE (region, id);


--
-- Name: events_leaf events_leaf_pkey1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_leaf
    ADD CONSTRAINT events_leaf_pkey1 PRIMARY KEY (id);


--
-- Name: events_leaf events_leaf_region_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_leaf
    ADD CONSTRAINT events_leaf_region_id_key UNIQUE (region, id);


--
-- Name: events_leaf_pkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_leaf_pkey ON ONLY public.events USING btree (id);


--
-- Name: events_archive_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_archive_id_idx ON ONLY public.events_archive USING btree (id);


--
-- Name: events_archive_old_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_archive_old_id_idx ON public.events_archive_old USING btree (id);


--
-- Name: events_archive_old_pkey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_archive_old_pkey ON public.events_archive_old USING btree (region);


--
-- Name: events_leaf_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX events_leaf_id_idx ON public.events_leaf USING btree (id);


--
-- Name: events_archive_id_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_leaf_pkey ATTACH PARTITION public.events_archive_id_idx;


--
-- Name: events_archive_old_id_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_archive_id_idx ATTACH PARTITION public.events_archive_old_id_idx;


--
-- Name: events_archive_old_pk; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_archive_pkey ATTACH PARTITION public.events_archive_old_pk;


--
-- Name: events_archive_old_region_id_key; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_archive_region_id_key ATTACH PARTITION public.events_archive_old_region_id_key;


--
-- Name: events_archive_pkey; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_pkey ATTACH PARTITION public.events_archive_pkey;


--
-- Name: events_archive_region_id_key; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_region_id_key ATTACH PARTITION public.events_archive_region_id_key;


--
-- Name: events_leaf_id_idx; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_leaf_pkey ATTACH PARTITION public.events_leaf_id_idx;


--
-- Name: events_leaf_pkey1; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_pkey ATTACH PARTITION public.events_leaf_pkey1;


--
-- Name: events_leaf_region_id_key; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_region_id_key ATTACH PARTITION public.events_leaf_region_id_key;


--
-- Name: event_notes event_notes_event_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.event_notes
    ADD CONSTRAINT event_notes_event_id_fkey FOREIGN KEY (event_id) REFERENCES public.events(id);


--
-- PostgreSQL database dump complete
--


