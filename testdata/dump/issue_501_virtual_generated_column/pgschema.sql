--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.11.1


--
-- Name: vt; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS vt (
    slug text NOT NULL,
    identifier text GENERATED ALWAYS AS (('urn:sdp:catalog:'::text || slug)) VIRTUAL
);

