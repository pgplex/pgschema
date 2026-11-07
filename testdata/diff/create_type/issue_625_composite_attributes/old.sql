CREATE TYPE example_input AS (
    id integer
);

CREATE TYPE address AS (
    street text,
    zip integer,
    country text
);

CREATE TABLE customer (
    id integer PRIMARY KEY,
    addr address
);
