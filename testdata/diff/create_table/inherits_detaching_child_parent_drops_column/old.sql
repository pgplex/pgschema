CREATE TABLE vehicles (
    id integer NOT NULL,
    wheels integer
);

CREATE TABLE cars (
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);
