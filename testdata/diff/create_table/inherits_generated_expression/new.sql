CREATE TABLE measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED
);

-- The child keeps the column generated but with its own expression
CREATE TABLE calibrated (
    scaled integer GENERATED ALWAYS AS (raw * 10 + 1) STORED,
    device text
) INHERITS (measurements);
