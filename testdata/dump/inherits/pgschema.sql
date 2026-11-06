--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 18.3
-- Dumped by pgschema version 1.14.0


--
-- Name: audited; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS audited (
    created_at timestamptz DEFAULT now() NOT NULL,
    created_by text
);

--
-- Name: cities; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS cities (
    name text,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_not_capital CHECK (name <> 'Capital'::text) NO INHERIT,
    CONSTRAINT cities_pop_check CHECK (population >= 0::double precision)
);


COMMENT ON TABLE cities IS 'Every city';


COMMENT ON COLUMN cities.population IS 'Residents, in thousands';

--
-- Name: capitals; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS capitals (
    elevation integer DEFAULT 0 NOT NULL,
    state character(2) NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> ''::bpchar)
) INHERITS (cities);
ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;


COMMENT ON COLUMN capitals.state IS 'Two-letter state code';

--
-- Name: capitals_state_idx; Type: INDEX; Schema: -; Owner: -
--

CREATE INDEX IF NOT EXISTS capitals_state_idx ON capitals (state);

--
-- Name: located; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK (region <> ''::text)
);

--
-- Name: orders; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    amount numeric(10,2),
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_amount_check CHECK (amount > 0::numeric)
) INHERITS (audited, located);

--
-- Name: state_capitals; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS state_capitals (
    founded date
) INHERITS (capitals);

