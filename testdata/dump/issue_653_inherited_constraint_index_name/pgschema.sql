--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.13.4


--
-- Name: events; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events (
    id integer NOT NULL,
    region text NOT NULL
) PARTITION BY RANGE (id);

--
-- Name: event_notes; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS event_notes (
    event_id integer NOT NULL,
    note text
);

--
-- Name: events_archive; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events_archive PARTITION OF events FOR VALUES FROM (10) TO (20) PARTITION BY RANGE (id);

--
-- Name: events_archive_old; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events_archive_old PARTITION OF events_archive FOR VALUES FROM (10) TO (15);

--
-- Name: events_archive_old_id_idx; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS events_archive_old_id_idx ON events_archive_old (id);

--
-- Name: events_archive_old_pkey; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS events_archive_old_pkey ON events_archive_old (region);

--
-- Name: events_leaf; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

--
-- Name: events_leaf_id_idx; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS events_leaf_id_idx ON events_leaf (id);

--
-- Name: events_pkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE ONLY events ADD CONSTRAINT events_pkey PRIMARY KEY (id);
ALTER TABLE ONLY events_archive ADD CONSTRAINT events_archive_pkey PRIMARY KEY (id);
ALTER TABLE ONLY events_archive_old ADD CONSTRAINT events_archive_old_pk PRIMARY KEY (id);
ALTER INDEX events_archive_pkey ATTACH PARTITION events_archive_old_pk;
ALTER INDEX events_pkey ATTACH PARTITION events_archive_pkey;
ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_pkey1 PRIMARY KEY (id);
ALTER INDEX events_pkey ATTACH PARTITION events_leaf_pkey1;

--
-- Name: events_region_id_key; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE ONLY events ADD CONSTRAINT events_region_id_key UNIQUE (region, id);
ALTER TABLE ONLY events_archive ADD CONSTRAINT events_archive_region_id_key UNIQUE (region, id);
ALTER TABLE ONLY events_archive_old ADD CONSTRAINT events_archive_old_region_id_key UNIQUE (region, id);
ALTER INDEX events_archive_region_id_key ATTACH PARTITION events_archive_old_region_id_key;
ALTER INDEX events_region_id_key ATTACH PARTITION events_archive_region_id_key;
ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_region_id_key UNIQUE (region, id);
ALTER INDEX events_region_id_key ATTACH PARTITION events_leaf_region_id_key;

--
-- Name: upsert_event(integer, text); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION upsert_event(
    p_id integer,
    p_region text
)
RETURNS void
LANGUAGE sql
VOLATILE
BEGIN ATOMIC
 INSERT INTO events (id, region)
   VALUES (upsert_event.p_id, upsert_event.p_region) ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET region = excluded.region;
END;

--
-- Name: events_archive_id_idx; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS events_archive_id_idx ON ONLY events_archive (id);

--
-- Name: events_archive_id_idx; Type: INDEX; Schema: -; Owner: -
--

ALTER INDEX events_archive_id_idx ATTACH PARTITION events_archive_old_id_idx;

--
-- Name: events_leaf_pkey; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS events_leaf_pkey ON ONLY events (id);

--
-- Name: events_leaf_pkey; Type: INDEX; Schema: -; Owner: -
--

ALTER INDEX events_leaf_pkey ATTACH PARTITION events_archive_id_idx;

--
-- Name: events_leaf_pkey; Type: INDEX; Schema: -; Owner: -
--

ALTER INDEX events_leaf_pkey ATTACH PARTITION events_leaf_id_idx;

--
-- Name: event_notes_event_id_fkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE event_notes
ADD CONSTRAINT event_notes_event_id_fkey FOREIGN KEY (event_id) REFERENCES events (id);

