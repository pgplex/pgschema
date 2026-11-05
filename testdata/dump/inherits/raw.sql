--
-- Regular table inheritance (CREATE TABLE ... INHERITS)
--

CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0),
    CONSTRAINT cities_not_capital CHECK (name <> 'Capital') NO INHERIT
);

COMMENT ON TABLE cities IS 'Every city';
COMMENT ON COLUMN cities.population IS 'Residents, in thousands';

-- elevation is declared again: PostgreSQL merges it with the parent's column
-- and keeps the child's DEFAULT and NOT NULL
CREATE TABLE capitals (
    state char(2) NOT NULL,
    elevation integer DEFAULT 0 NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> '')
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

COMMENT ON COLUMN capitals.state IS 'Two-letter state code';

CREATE INDEX capitals_state_idx ON capitals (state);

-- A grandchild, inheriting through capitals
CREATE TABLE state_capitals (
    founded date
) INHERITS (capitals);

-- Multiple parents
CREATE TABLE audited (
    created_at timestamptz DEFAULT now() NOT NULL,
    created_by text
);

CREATE TABLE located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK (region <> '')
);

CREATE TABLE orders (
    id integer NOT NULL,
    amount numeric(10,2),
    CONSTRAINT orders_pkey PRIMARY KEY (id),
    CONSTRAINT orders_amount_check CHECK (amount > 0)
) INHERITS (audited, located);
