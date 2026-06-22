--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.11.1


--
-- Name: items; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS items (
    id integer,
    name varchar(64),
    tags varchar(128)[],
    codes character(10)[],
    CONSTRAINT items_pkey PRIMARY KEY (id)
);

