CREATE TABLE IF NOT EXISTS sensor_a (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED
);

CREATE TABLE IF NOT EXISTS sensor_b (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 20)) STORED
);

CREATE TABLE IF NOT EXISTS combined (
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED,
    label text
) INHERITS (sensor_a, sensor_b);
