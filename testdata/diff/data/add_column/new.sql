CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL,
    region text NOT NULL DEFAULT 'unknown'
);

\copy country (code, name, region) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
