--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.0
-- Dumped by pgschema version 1.11.1


--
-- Name: ledger; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger (
    id uuid,
    amount bigint NOT NULL,
    ts timestamptz,
    CONSTRAINT ledger_pkey PRIMARY KEY (ts, id)
) PARTITION BY RANGE (ts);

--
-- Name: ledger_2026_06; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS ledger_2026_06 (
    id uuid,
    amount bigint NOT NULL,
    ts timestamptz,
    CONSTRAINT ledger_2026_06_pkey PRIMARY KEY (ts, id)
);

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

