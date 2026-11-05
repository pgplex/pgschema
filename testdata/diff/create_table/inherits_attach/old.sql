CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0)
);

-- A standalone table with the parent's columns but not its CHECK constraint
CREATE TABLE capitals (
    name text NOT NULL,
    population real,
    elevation integer,
    state char(2) NOT NULL
);
