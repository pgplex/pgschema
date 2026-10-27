--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.1
-- Dumped by pgschema version 1.13.4


--
-- Name: p; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS p (
    a integer,
    b integer,
    CONSTRAINT p_pkey PRIMARY KEY (a, b),
    CONSTRAINT p_b_a_key UNIQUE (b, a)
);

--
-- Name: c1; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS c1 (
    a integer,
    b integer,
    CONSTRAINT fk_p FOREIGN KEY (a, b) REFERENCES p (a, b)
);

--
-- Name: c2; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS c2 (
    a integer,
    b integer,
    CONSTRAINT fk_p FOREIGN KEY (b, a) REFERENCES p (b, a)
);

