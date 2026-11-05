CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0)
);

CREATE TABLE capitals (
    state char(2) NOT NULL
) INHERITS (cities);
