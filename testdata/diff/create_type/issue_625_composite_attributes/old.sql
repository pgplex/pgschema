CREATE TYPE legacy_flag AS ENUM (
    'x'
);

CREATE TYPE example_input AS (
    id integer
);

CREATE TYPE address AS (
    street text,
    zip integer,
    country text,
    flag legacy_flag
);

CREATE TABLE customer (
    id integer PRIMARY KEY,
    addr address
);
