-- wheels moves from the parent to cars alone: the parent drops it with ONLY
-- so cars keeps the column and its data, and boats drops its own copy.
CREATE TABLE vehicles (
    id integer NOT NULL,
    name text
);

CREATE TABLE cars (
    wheels integer,
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);
