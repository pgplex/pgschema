ALTER TABLE case_notes DROP CONSTRAINT case_notes_case_id_fkey;

ALTER TABLE case_tags DROP CONSTRAINT case_tags_case_id_fkey;

ALTER TABLE cases DROP CONSTRAINT cases_id_key;

ALTER TABLE cases
ADD CONSTRAINT cases_pkey PRIMARY KEY (id);

ALTER TABLE case_notes
ADD CONSTRAINT case_notes_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id) NOT VALID;

ALTER TABLE case_notes VALIDATE CONSTRAINT case_notes_case_id_fkey;

ALTER TABLE case_tags
ADD CONSTRAINT case_tags_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id) ON DELETE CASCADE NOT VALID;

ALTER TABLE case_tags VALIDATE CONSTRAINT case_tags_case_id_fkey;
