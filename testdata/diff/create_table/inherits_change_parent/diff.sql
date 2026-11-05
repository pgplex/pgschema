ALTER TABLE child NO INHERIT base_a;

ALTER TABLE child DROP COLUMN label_a;

ALTER TABLE child ADD COLUMN label_b text;

ALTER TABLE ONLY child ADD CONSTRAINT base_b_id_check CHECK (id > 0);

ALTER TABLE child INHERIT base_b;
