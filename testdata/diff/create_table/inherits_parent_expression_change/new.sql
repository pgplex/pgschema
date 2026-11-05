-- The parent's new expression recurses to calibrated, whose own expression
-- is then put back.
CREATE TABLE measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 4) STORED
);

CREATE TABLE calibrated (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (measurements);
