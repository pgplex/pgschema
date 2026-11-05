CREATE TABLE IF NOT EXISTS measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED
);

CREATE TABLE IF NOT EXISTS calibrated (
    scaled integer GENERATED ALWAYS AS (((raw * 10) + 1)) STORED,
    device text
) INHERITS (measurements);
