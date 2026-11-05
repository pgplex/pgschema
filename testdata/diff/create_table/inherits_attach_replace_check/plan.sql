ALTER TABLE archived_readings DROP CONSTRAINT readings_value_check;

ALTER TABLE archived_readings
ADD CONSTRAINT readings_value_check CHECK (value > 10) NOT VALID;

ALTER TABLE archived_readings VALIDATE CONSTRAINT readings_value_check;

ALTER TABLE archived_readings INHERIT readings;
