--
-- PostgreSQL database dump
--

SET statement_timeout = 0;
SET lock_timeout = 0;
-- SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: ledger; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ledger (
    id uuid NOT NULL,
    amount bigint NOT NULL,
    ts timestamp with time zone NOT NULL
)
PARTITION BY RANGE (ts);

ALTER TABLE ONLY public.ledger
    ADD CONSTRAINT ledger_pkey PRIMARY KEY (ts, id);

--
-- Name: ledger_2026_06; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ledger_2026_06 PARTITION OF public.ledger
FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

--
-- Name: tg_noop(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.tg_noop() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RETURN NEW; END $$;

--
-- Name: ledger trg_rollup; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_rollup AFTER INSERT ON public.ledger FOR EACH ROW EXECUTE FUNCTION public.tg_noop();

--
-- PostgreSQL database dump complete
--
