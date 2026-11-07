CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

ALTER TYPE address DROP ATTRIBUTE country;

ALTER TYPE address ADD ATTRIBUTE city text;

ALTER TYPE example_input ALTER ATTRIBUTE id TYPE bigint;

ALTER TYPE example_input ADD ATTRIBUTE label text;

ALTER TYPE example_input ADD ATTRIBUTE prio priority;
