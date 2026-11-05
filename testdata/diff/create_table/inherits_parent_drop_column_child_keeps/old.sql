CREATE TABLE vehicles (
    id integer NOT NULL,
    wheels integer,
    name text
);

CREATE TABLE cars (
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);
