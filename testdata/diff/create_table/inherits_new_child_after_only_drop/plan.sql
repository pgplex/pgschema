CREATE TABLE IF NOT EXISTS boats (
    draft numeric
) INHERITS (vehicles);

ALTER TABLE ONLY vehicles DROP COLUMN wheels;

ALTER TABLE boats DROP COLUMN wheels;
