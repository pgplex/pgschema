ALTER TABLE child NO INHERIT base_b;

ALTER TABLE base_a ADD COLUMN added text;

ALTER TABLE child DROP COLUMN label_b;
