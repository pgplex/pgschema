--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.11.0


--
-- Name: session; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session (
    id bigint,
    started_at timestamptz,
    CONSTRAINT session_pkey PRIMARY KEY (started_at, id)
) PARTITION BY RANGE (started_at);

--
-- Name: event; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS event (
    session_id bigint NOT NULL,
    session_started_at timestamptz NOT NULL,
    CONSTRAINT event_session_id_session_started_at_fkey FOREIGN KEY (session_id, session_started_at) REFERENCES session (id, started_at)
);

--
-- Name: session_2026_01; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_01 (
    id bigint,
    started_at timestamptz,
    CONSTRAINT session_2026_01_pkey PRIMARY KEY (started_at, id)
);

--
-- Name: session_2026_02; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS session_2026_02 (
    id bigint,
    started_at timestamptz,
    CONSTRAINT session_2026_02_pkey PRIMARY KEY (started_at, id)
);

