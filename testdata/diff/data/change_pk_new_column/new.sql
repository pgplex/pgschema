CREATE TABLE country (
    code uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL
);

\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
