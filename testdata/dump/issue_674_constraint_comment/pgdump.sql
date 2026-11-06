--
-- PostgreSQL database dump
--


-- Dumped from database version 17.6 (Homebrew)
-- Dumped by pg_dump version 17.6 (Homebrew)

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
-- Name: customers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.customers (
    id integer NOT NULL,
    email text
);


--
-- Name: events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events (
    id integer NOT NULL
)
PARTITION BY RANGE (id);


--
-- Name: events_leaf; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.events_leaf (
    id integer NOT NULL
);


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    id integer NOT NULL,
    customer_id integer NOT NULL,
    total numeric NOT NULL,
    CONSTRAINT orders_total_check CHECK ((total >= (0)::numeric))
);


--
-- Name: CONSTRAINT orders_total_check ON orders; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT orders_total_check ON public.orders IS 'totals are never negative';


--
-- Name: events_leaf; Type: TABLE ATTACH; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events ATTACH PARTITION public.events_leaf FOR VALUES FROM (0) TO (10);


--
-- Name: customers customers_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_email_key UNIQUE (email);


--
-- Name: CONSTRAINT customers_email_key ON customers; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT customers_email_key ON public.customers IS 'one account per email';


--
-- Name: customers customers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.customers
    ADD CONSTRAINT customers_pkey PRIMARY KEY (id);


--
-- Name: events events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events
    ADD CONSTRAINT events_pkey PRIMARY KEY (id);


--
-- Name: CONSTRAINT events_pkey ON events; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT events_pkey ON public.events IS 'parent key';


--
-- Name: events_leaf events_leaf_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.events_leaf
    ADD CONSTRAINT events_leaf_pkey PRIMARY KEY (id);


--
-- Name: CONSTRAINT events_leaf_pkey ON events_leaf; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT events_leaf_pkey ON public.events_leaf IS 'child key';


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (id);


--
-- Name: CONSTRAINT orders_pkey ON orders; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT orders_pkey ON public.orders IS 'order identifier';


--
-- Name: events_leaf_pkey; Type: INDEX ATTACH; Schema: public; Owner: -
--

ALTER INDEX public.events_pkey ATTACH PARTITION public.events_leaf_pkey;


--
-- Name: orders orders_customer_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES public.customers(id);


--
-- Name: CONSTRAINT orders_customer_id_fkey ON orders; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT orders_customer_id_fkey ON public.orders IS 'owning customer';


--
-- PostgreSQL database dump complete
--

