CREATE TABLE country (
    id integer PRIMARY KEY,
    code text NOT NULL UNIQUE,
    name text NOT NULL
);

INSERT INTO country (id, code, name) VALUES (1, 'IL', 'Israel'), (2, 'US', 'United States'), (3, 'XX', 'Neutral Zone');
