CREATE TYPE period AS RANGE (
    subtype = date
);

CREATE DOMAIN availability AS period_multirange;

CREATE TYPE booking AS (room integer, slot period);
