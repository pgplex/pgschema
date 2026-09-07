CREATE TABLE country (
    id integer PRIMARY KEY,
    code text NOT NULL UNIQUE,
    name text NOT NULL
);

CREATE TABLE city (
    name text PRIMARY KEY,
    country_code text NOT NULL REFERENCES country(code)
);

INSERT INTO country (id, code, name) VALUES (1, 'IL', 'Israel'), (2, 'US', 'United States');
INSERT INTO city (name, country_code) VALUES ('Tel Aviv', 'IL'), ('Austin', 'US');
