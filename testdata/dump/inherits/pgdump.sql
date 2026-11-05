--
-- PostgreSQL database dump
--

-- Dumped from database version 17.5
-- Dumped by pg_dump version 17.5 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
-- SET transaction_timeout = 0;
SET client_encoding = 'SQL_ASCII';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: audited; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audited (
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by text
);


--
-- Name: cities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_not_capital CHECK ((name <> 'Capital'::text)) NO INHERIT,
    CONSTRAINT cities_pop_check CHECK ((population >= (0)::double precision))
);


--
-- Name: TABLE cities; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.cities IS 'Every city';


--
-- Name: COLUMN cities.population; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.cities.population IS 'Residents, in thousands';


--
-- Name: capitals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.capitals (
    elevation integer DEFAULT 0 NOT NULL,
    state character(2) NOT NULL,
    CONSTRAINT capitals_state_check CHECK ((state <> ''::bpchar))
)
INHERITS (public.cities);


--
-- Name: COLUMN capitals.state; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.capitals.state IS 'Two-letter state code';


--
-- Name: located; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK ((region <> ''::text))
);


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    id integer NOT NULL,
    amount numeric(10,2),
    CONSTRAINT orders_amount_check CHECK ((amount > (0)::numeric))
)
INHERITS (public.audited, public.located);


--
-- Name: state_capitals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.state_capitals (
    founded date
)
INHERITS (public.capitals);


--
-- Name: capitals population; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.capitals ALTER COLUMN population SET DEFAULT 1;


--
-- Name: orders created_at; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders ALTER COLUMN created_at SET DEFAULT now();


--
-- Name: state_capitals population; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.state_capitals ALTER COLUMN population SET DEFAULT 1;


--
-- Name: state_capitals elevation; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.state_capitals ALTER COLUMN elevation SET DEFAULT 0;


--
-- Name: cities cities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cities
    ADD CONSTRAINT cities_pkey PRIMARY KEY (name);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: capitals_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX capitals_state_idx ON public.capitals USING btree (state);


--
-- PostgreSQL database dump complete
--

