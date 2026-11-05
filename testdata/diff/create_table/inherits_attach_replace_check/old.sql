CREATE TABLE readings (
    value integer,
    CONSTRAINT readings_value_check CHECK (value > 10)
);

-- A standalone table whose same-named CHECK differs from the parent's
CREATE TABLE archived_readings (
    value integer,
    archived_at timestamptz,
    CONSTRAINT readings_value_check CHECK (value > 0)
);
