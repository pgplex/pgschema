CREATE TABLE country (
    id integer PRIMARY KEY,
    code text NOT NULL,
    region text,
    UNIQUE NULLS NOT DISTINCT (code, region)
);

\copy country (id, code, region) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
