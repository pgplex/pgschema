ALTER TABLE case_notes DROP CONSTRAINT case_notes_case_id_fkey;

ALTER TABLE cases DROP CONSTRAINT cases_id_key;

ALTER TABLE cases
ADD CONSTRAINT cases_pkey PRIMARY KEY (id);

ALTER TABLE case_notes
ADD CONSTRAINT case_notes_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id) ON DELETE CASCADE;
