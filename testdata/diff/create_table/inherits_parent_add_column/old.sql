CREATE TABLE cities (
    name text NOT NULL,
    population real,
    CONSTRAINT cities_pkey PRIMARY KEY (name)
);

CREATE TABLE capitals (
    state char(2) NOT NULL
) INHERITS (cities);
