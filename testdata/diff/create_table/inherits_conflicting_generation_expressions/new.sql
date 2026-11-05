CREATE TABLE sensor_a (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED
);

CREATE TABLE sensor_b (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 20) STORED
);

-- The parents generate scaled differently; the child must say which, even
-- when it picks the first parent's expression.
CREATE TABLE combined (
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED,
    label text
) INHERITS (sensor_a, sensor_b);
