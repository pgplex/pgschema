CREATE TABLE cases (
    id integer PRIMARY KEY,
    name text
);

CREATE TABLE case_notes (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_notes_case_id_fkey REFERENCES cases (id)
);

CREATE TABLE case_tags (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_tags_case_id_fkey REFERENCES cases (id) ON DELETE CASCADE
);
