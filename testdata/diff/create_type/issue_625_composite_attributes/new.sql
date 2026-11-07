CREATE TYPE priority AS ENUM (
    'low',
    'high'
);

CREATE TYPE example_input AS (
    id bigint,
    label text,
    prio priority
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
