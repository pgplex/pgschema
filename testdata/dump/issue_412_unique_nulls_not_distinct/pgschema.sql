--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.9.0


--
-- Name: pgschema_repro_nulls; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS pgschema_repro_nulls (
    a integer,
    b integer,
    CONSTRAINT pgschema_repro_nulls_uniq UNIQUE NULLS NOT DISTINCT (a, b)
);

