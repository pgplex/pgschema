ALTER TABLE cities DROP CONSTRAINT cities_pop_check;

ALTER TABLE cities DROP COLUMN elevation;

ALTER TABLE cities ALTER COLUMN population TYPE bigint USING population::bigint;
