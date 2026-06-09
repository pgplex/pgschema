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
-- Name: session; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.session (
    id bigint NOT NULL,
    started_at timestamp with time zone NOT NULL
)
PARTITION BY RANGE (started_at);

ALTER TABLE ONLY public.session
    ADD CONSTRAINT session_pkey PRIMARY KEY (id, started_at);

--
-- Name: event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.event (
    session_id bigint NOT NULL,
    session_started_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.event
    ADD CONSTRAINT event_session_id_session_started_at_fkey FOREIGN KEY (session_id, session_started_at) REFERENCES public.session(id, started_at);

--
-- Name: session_2026_01; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.session_2026_01 PARTITION OF public.session
FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

--
-- Name: session_2026_02; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.session_2026_02 PARTITION OF public.session
FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');

--
-- PostgreSQL database dump complete
--
