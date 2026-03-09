--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.7.3


--
-- Name: repro; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS repro (
    id uuid DEFAULT gen_random_uuid(),
    data jsonb DEFAULT '{}',
    CONSTRAINT repro_pkey PRIMARY KEY (id)
);

--
-- Name: repro; Type: RLS; Schema: -; Owner: -
--

ALTER TABLE repro ENABLE ROW LEVEL SECURITY;

--
-- Name: p; Type: POLICY; Schema: -; Owner: -
--

CREATE POLICY p ON repro TO PUBLIC USING ((data #>> '{nested,key}'::text[]) = 'x');

