CREATE TABLE IF NOT EXISTS case_files (
    id integer,
    case_id integer,
    CONSTRAINT case_files_pkey PRIMARY KEY (id)
);

ALTER TABLE case_comments DROP CONSTRAINT case_comments_case_id_fkey;

ALTER TABLE case_notes DROP CONSTRAINT case_notes_case_id_fkey;

ALTER TABLE case_tags DROP CONSTRAINT case_tags_case_id_fkey;

ALTER TABLE cases DROP CONSTRAINT cases_id_key;

ALTER TABLE cases
ADD CONSTRAINT cases_pkey PRIMARY KEY (id);

ALTER TABLE case_comments
ADD CONSTRAINT case_comments_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id) ON DELETE CASCADE;

ALTER TABLE case_files
ADD CONSTRAINT case_files_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id);

ALTER TABLE case_notes
ADD CONSTRAINT case_notes_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id);

ALTER TABLE case_tags
ADD CONSTRAINT case_tags_case_id_fkey FOREIGN KEY (case_id) REFERENCES cases (id) ON DELETE CASCADE;
