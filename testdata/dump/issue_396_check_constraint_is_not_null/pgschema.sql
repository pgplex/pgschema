--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.9.0


--
-- Name: test_table; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS test_table (
    id integer,
    status text NOT NULL,
    reason text,
    actor_id uuid,
    CONSTRAINT test_table_pkey PRIMARY KEY (id),
    CONSTRAINT test_table_status_check CHECK (status = 'active'::text OR status = 'cancelled'::text AND reason IS NOT NULL OR status = 'revoked'::text AND actor_id IS NOT NULL)
);

