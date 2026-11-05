ALTER TABLE archived_readings DROP CONSTRAINT readings_value_check;

ALTER TABLE archived_readings ADD CONSTRAINT readings_value_check CHECK (value > 10);

ALTER TABLE archived_readings INHERIT readings;
