--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.7.2


--
-- Name: base_data; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS base_data (
    id integer,
    value text NOT NULL,
    category text,
    CONSTRAINT base_data_pkey PRIMARY KEY (id)
);

--
-- Name: item_summary; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW item_summary AS
 SELECT id,
    value,
    category
   FROM base_data
  WHERE category IS NOT NULL;

--
-- Name: dashboard; Type: VIEW; Schema: -; Owner: -
--

CREATE OR REPLACE VIEW dashboard AS
 SELECT id,
    value
   FROM item_summary;

