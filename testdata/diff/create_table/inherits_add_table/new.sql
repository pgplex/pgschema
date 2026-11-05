CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0)
);

-- elevation is declared again in the child: PostgreSQL merges it with the
-- parent's column, keeping the child's DEFAULT and NOT NULL.
CREATE TABLE capitals (
    state char(2) NOT NULL,
    elevation integer DEFAULT 0 NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> '')
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

CREATE INDEX capitals_state_idx ON capitals (state);
