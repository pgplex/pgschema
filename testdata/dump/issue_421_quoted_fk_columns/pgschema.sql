--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.5
-- Dumped by pgschema version 1.11.0


--
-- Name: aaa; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS aaa (
    "aId" bigint,
    "bId" bigint,
    CONSTRAINT aaa_pkey PRIMARY KEY ("aId")
);

--
-- Name: bbb; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS bbb (
    "bId" bigint,
    "aId" bigint,
    CONSTRAINT bbb_pkey PRIMARY KEY ("bId"),
    CONSTRAINT bbb_fk FOREIGN KEY ("aId") REFERENCES aaa ("aId") DEFERRABLE
);

--
-- Name: aaa_fk; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE aaa
ADD CONSTRAINT aaa_fk FOREIGN KEY ("bId") REFERENCES bbb ("bId") DEFERRABLE;

