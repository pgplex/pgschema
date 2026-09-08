CREATE TABLE country (
    id integer PRIMARY KEY,
    code text NOT NULL UNIQUE,
    name text NOT NULL
);

\copy country (id, code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
