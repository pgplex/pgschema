CREATE TABLE readings (
    value integer,
    CONSTRAINT readings_value_check CHECK (value > 10)
);

CREATE TABLE archived_readings (
    archived_at timestamptz
) INHERITS (readings);
