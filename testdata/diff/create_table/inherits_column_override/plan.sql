ALTER TABLE capitals ADD CONSTRAINT capitals_population_not_null NOT NULL population NOT VALID;

ALTER TABLE capitals VALIDATE CONSTRAINT capitals_population_not_null;

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

ALTER TABLE ONLY capitals ALTER COLUMN elevation DROP DEFAULT;
