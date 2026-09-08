CREATE TABLE country (
    code text PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE city (
    name text PRIMARY KEY,
    country_code text NOT NULL REFERENCES country(code)
);

INSERT INTO country (code, name) VALUES ('IL', 'Israel'), ('US', 'United States');
INSERT INTO city (name, country_code) VALUES ('Tel Aviv', 'IL'), ('Austin', 'US');
