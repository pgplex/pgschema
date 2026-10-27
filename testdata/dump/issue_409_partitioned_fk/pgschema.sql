--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.13.3


--
-- Name: session; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session (
    id bigint NOT NULL,
    started_at timestamptz NOT NULL
) PARTITION BY RANGE (started_at);

--
-- Name: event; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS event (
    session_id bigint NOT NULL,
    session_started_at timestamptz NOT NULL
);

--
-- Name: session_2026_01; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_01 PARTITION OF session FOR VALUES FROM ('2026-01-01 00:00:00+00') TO ('2026-02-01 00:00:00+00');

--
-- Name: session_2026_02; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_02 PARTITION OF session FOR VALUES FROM ('2026-02-01 00:00:00+00') TO ('2026-03-01 00:00:00+00');

--
-- Name: session_pkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE ONLY session ADD CONSTRAINT session_pkey PRIMARY KEY (id, started_at);
ALTER TABLE ONLY session_2026_01 ADD CONSTRAINT session_2026_01_pkey PRIMARY KEY (id, started_at);
ALTER INDEX session_pkey ATTACH PARTITION session_2026_01_pkey;
ALTER TABLE ONLY session_2026_02 ADD CONSTRAINT session_2026_02_pkey PRIMARY KEY (id, started_at);
ALTER INDEX session_pkey ATTACH PARTITION session_2026_02_pkey;

--
-- Name: event_session_id_session_started_at_fkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE event
ADD CONSTRAINT event_session_id_session_started_at_fkey FOREIGN KEY (session_id, session_started_at) REFERENCES session (id, started_at);

