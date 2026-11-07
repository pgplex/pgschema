CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

CREATE TABLE region (
    code text PRIMARY KEY
);

CREATE DOMAIN region_row AS region;

CREATE FUNCTION is_positive(n integer) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT n > 0 $$;

CREATE DOMAIN positive_int AS integer CHECK (is_positive(VALUE));

CREATE TYPE example_input AS (
    id bigint,
    label text,
    prio priority,
    region region_row,
    quantity positive_int
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
