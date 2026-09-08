CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL,
    active boolean NOT NULL DEFAULT true
);

\copy country (code, name, active) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
