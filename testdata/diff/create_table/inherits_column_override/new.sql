CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name)
);

CREATE TABLE capitals (
    state char(2) NOT NULL
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;
ALTER TABLE ONLY capitals ALTER COLUMN population SET NOT NULL;
