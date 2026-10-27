--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.13.3


--
-- Name: ledger; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger (
    id uuid NOT NULL,
    amount bigint NOT NULL,
    ts timestamptz NOT NULL
) PARTITION BY RANGE (ts);

--
-- Name: ledger_2026_06; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger_2026_06 PARTITION OF ledger FOR VALUES FROM ('2026-06-01 00:00:00+00') TO ('2026-07-01 00:00:00+00');

--
-- Name: ledger_pkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE ONLY ledger ADD CONSTRAINT ledger_pkey PRIMARY KEY (ts, id);
ALTER TABLE ONLY ledger_2026_06 ADD CONSTRAINT ledger_2026_06_pkey PRIMARY KEY (ts, id);
ALTER INDEX ledger_pkey ATTACH PARTITION ledger_2026_06_pkey;

--
-- Name: tg_noop(); Type: FUNCTION; Schema: -; Owner: -
--

CREATE OR REPLACE FUNCTION tg_noop()
RETURNS trigger
LANGUAGE plpgsql
VOLATILE
AS $$ BEGIN RETURN NEW; END
$$;

--
-- Name: trg_rollup; Type: TRIGGER; Schema: -; Owner: -
--

CREATE OR REPLACE TRIGGER trg_rollup
    AFTER INSERT ON ledger
    FOR EACH ROW
    EXECUTE FUNCTION tg_noop();

