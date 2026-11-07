ALTER TYPE address DROP ATTRIBUTE country;

ALTER TYPE address DROP ATTRIBUTE flag;

DROP TYPE IF EXISTS legacy_flag RESTRICT;

CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

ALTER TYPE address ADD ATTRIBUTE city text;

ALTER TYPE example_input ALTER ATTRIBUTE id TYPE bigint;

ALTER TYPE example_input ADD ATTRIBUTE label text;

ALTER TYPE example_input ADD ATTRIBUTE prio priority;

CREATE TABLE IF NOT EXISTS region (
    code text,
    CONSTRAINT region_pkey PRIMARY KEY (code)
);

CREATE OR REPLACE FUNCTION is_positive(
    n integer
)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$ SELECT n > 0
$$;

CREATE DOMAIN positive_int AS integer
  CONSTRAINT positive_int_check CHECK (is_positive(VALUE));

ALTER TYPE example_input ADD ATTRIBUTE quantity positive_int;

CREATE OR REPLACE FUNCTION get_quantity(
    p example_input
)
RETURNS integer
LANGUAGE sql
VOLATILE
RETURN (p).quantity;

CREATE DOMAIN region_row AS region;

ALTER TYPE example_input ADD ATTRIBUTE region region_row;

CREATE OR REPLACE FUNCTION has_region(
    p example_input
)
RETURNS boolean
LANGUAGE sql
VOLATILE
RETURN ((p).region IS NOT NULL);
