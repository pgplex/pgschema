--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.13.0


--
-- Name: recipient_search; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS recipient_search (
    id bigint,
    search_text text COLLATE "C" NOT NULL,
    display_name varchar(100) COLLATE "POSIX" DEFAULT '',
    note text,
    region text COLLATE my_coll,
    CONSTRAINT recipient_search_pkey PRIMARY KEY (id)
);

--
-- Name: recipient_search_prefix_idx; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS recipient_search_prefix_idx ON recipient_search (search_text text_pattern_ops);

