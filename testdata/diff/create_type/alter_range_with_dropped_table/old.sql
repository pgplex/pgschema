CREATE TYPE span AS RANGE (subtype = integer);

CREATE TABLE windows (
    id integer PRIMARY KEY,
    during span
);
