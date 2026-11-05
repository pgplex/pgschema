CREATE TABLE cities (
    name text NOT NULL,
    population real,
    country text DEFAULT 'US' NOT NULL,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0)
);

-- The child gets country and cities_pop_check from the parent; only its own
-- index on the new column needs a statement.
CREATE TABLE capitals (
    state char(2) NOT NULL
) INHERITS (cities);

CREATE INDEX capitals_country_idx ON capitals (country);
