CREATE TABLE country (
    id integer PRIMARY KEY,
    code text NOT NULL,
    region text,
    UNIQUE NULLS NOT DISTINCT (code, region)
);

INSERT INTO country (id, code, region) VALUES (1, 'IL', NULL), (2, 'US', NULL);
