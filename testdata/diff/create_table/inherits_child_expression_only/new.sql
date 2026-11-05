-- Only calibrated's expression changes; its own child keeps raw * 3, so the
-- change must not recurse.
CREATE TABLE measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 2) STORED
);

CREATE TABLE calibrated (
    scaled integer GENERATED ALWAYS AS (raw * 4) STORED
) INHERITS (measurements);

CREATE TABLE recalibrated (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (calibrated);
