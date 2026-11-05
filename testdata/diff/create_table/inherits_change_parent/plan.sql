ALTER TABLE child NO INHERIT base_a;

ALTER TABLE child DROP COLUMN label_a;

ALTER TABLE child ADD COLUMN label_b text;

ALTER TABLE child
ADD CONSTRAINT base_b_id_check CHECK (id > 0) NOT VALID;

ALTER TABLE child VALIDATE CONSTRAINT base_b_id_check;

ALTER TABLE child INHERIT base_b;
