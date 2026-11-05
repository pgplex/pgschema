-- cars keeps wheels, so vehicles drops it with ONLY; boats leaves vehicles
-- and drops wheels through its own changes.
CREATE TABLE vehicles (
    id integer NOT NULL
);

CREATE TABLE cars (
    wheels integer,
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    id integer NOT NULL,
    draft numeric
);
