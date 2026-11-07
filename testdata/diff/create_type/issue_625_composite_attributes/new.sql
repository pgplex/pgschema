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

CREATE FUNCTION get_quantity(p example_input) RETURNS integer
    LANGUAGE sql
    RETURN (p).quantity;

CREATE FUNCTION has_region(p example_input) RETURNS boolean
    LANGUAGE sql
    RETURN (p).region IS NOT NULL;

CREATE TYPE address AS (
    street text,
    zip integer,
    city text
);

CREATE TABLE customer (
    id integer PRIMARY KEY,
    addr address
);
