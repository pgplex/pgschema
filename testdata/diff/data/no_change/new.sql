CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL
);

\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
