CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE city (
    name text PRIMARY KEY,
    country_code text NOT NULL REFERENCES country(code)
);

\copy country (code, name) FROM 'data/country.csv' WITH (FORMAT csv, HEADER)
\copy city (name, country_code) FROM 'data/city.csv' WITH (FORMAT csv, HEADER)
