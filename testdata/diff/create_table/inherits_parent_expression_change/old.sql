CREATE TABLE measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 2) STORED
);

CREATE TABLE calibrated (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (measurements);
