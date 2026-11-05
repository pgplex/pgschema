-- cars keeps wheels, so vehicles drops it with ONLY. boats is created before
-- that and inherits wheels; it has to drop the column itself.
CREATE TABLE vehicles (
    id integer NOT NULL
);

CREATE TABLE cars (
    wheels integer,
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);
