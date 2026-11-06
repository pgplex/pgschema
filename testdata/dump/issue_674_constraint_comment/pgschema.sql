--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.14.0


--
-- Name: customers; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS customers (
    id integer,
    email text,
    CONSTRAINT customers_pkey PRIMARY KEY (id),
    CONSTRAINT customers_email_key UNIQUE (email)
);


COMMENT ON CONSTRAINT customers_email_key ON customers IS 'one account per email';

--
-- Name: events; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events (
    id integer NOT NULL
) PARTITION BY RANGE (id);

--
-- Name: events_leaf; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

--
-- Name: events_pkey; Type: CONSTRAINT; Schema: -; Owner: -
--

ALTER TABLE ONLY events ADD CONSTRAINT events_pkey PRIMARY KEY (id);
COMMENT ON CONSTRAINT events_pkey ON events IS 'parent key';
ALTER TABLE ONLY events_leaf ADD CONSTRAINT events_leaf_pkey PRIMARY KEY (id);
COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';
ALTER INDEX events_pkey ATTACH PARTITION events_leaf_pkey;

--
-- Name: orders; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    customer_id integer NOT NULL,
    total numeric NOT NULL,
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id),
    CONSTRAINT orders_total_check CHECK (total >= 0::numeric)
);


COMMENT ON CONSTRAINT orders_pkey ON orders IS 'order identifier';


COMMENT ON CONSTRAINT orders_customer_id_fkey ON orders IS 'owning customer';


COMMENT ON CONSTRAINT orders_total_check ON orders IS 'totals are never negative';

