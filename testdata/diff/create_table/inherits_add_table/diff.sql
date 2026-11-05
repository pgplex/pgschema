CREATE TABLE IF NOT EXISTS cities (
    name text,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0::double precision)
);

CREATE TABLE IF NOT EXISTS capitals (
    elevation integer DEFAULT 0 NOT NULL,
    state character(2) NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> ''::bpchar)
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

CREATE INDEX IF NOT EXISTS capitals_state_idx ON capitals (state);
