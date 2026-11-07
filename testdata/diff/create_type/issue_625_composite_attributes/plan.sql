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

CREATE DOMAIN region_row AS region;

ALTER TYPE example_input ADD ATTRIBUTE region region_row;
