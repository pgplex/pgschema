DROP TYPE IF EXISTS span RESTRICT;

DROP FUNCTION IF EXISTS int_diff(integer, integer);

CREATE TYPE span AS RANGE (
    subtype = bigint
);
