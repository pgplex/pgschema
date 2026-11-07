CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

CREATE TABLE region (
    code text PRIMARY KEY
);

CREATE DOMAIN region_row AS region;

CREATE TYPE example_input AS (
    id bigint,
    label text,
    prio priority,
    region region_row
);

CREATE TYPE address AS (
    street text,
    zip integer,
    city text
);

CREATE TABLE customer (
    id integer PRIMARY KEY,
    addr address
);
