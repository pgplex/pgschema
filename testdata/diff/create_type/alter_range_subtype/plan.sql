DROP TYPE IF EXISTS span RESTRICT;

CREATE TYPE span AS RANGE (
    subtype = bigint
);
