ALTER TABLE country ADD COLUMN region text DEFAULT 'unknown' NOT NULL;

UPDATE country SET region = 'EMEA' WHERE code = 'IL';

UPDATE country SET region = 'unknown' WHERE code = 'US';
