ALTER TABLE people ADD CONSTRAINT people_created_at_not_null NOT NULL created_at NOT VALID;

ALTER TABLE people VALIDATE CONSTRAINT people_created_at_not_null;

ALTER TABLE people ALTER COLUMN created_at SET DEFAULT now();
