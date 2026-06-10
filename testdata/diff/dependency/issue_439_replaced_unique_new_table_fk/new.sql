CREATE TABLE cases (
    id integer PRIMARY KEY
);
CREATE TABLE case_notes (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_notes_case_id_fkey REFERENCES cases (id)
);
