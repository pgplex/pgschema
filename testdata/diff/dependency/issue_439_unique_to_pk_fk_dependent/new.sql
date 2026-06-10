CREATE TABLE cases (
    id integer PRIMARY KEY,
    name text
);

-- unchanged FK
CREATE TABLE case_notes (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_notes_case_id_fkey REFERENCES cases (id)
);

-- unchanged FK with ON DELETE CASCADE (clause must survive recreation)
CREATE TABLE case_tags (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_tags_case_id_fkey REFERENCES cases (id) ON DELETE CASCADE
);

-- FK that itself changes in the same migration (gains ON DELETE CASCADE)
CREATE TABLE case_comments (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_comments_case_id_fkey REFERENCES cases (id) ON DELETE CASCADE
);

-- newly added table whose FK references the replaced constraint
CREATE TABLE case_files (
    id integer PRIMARY KEY,
    case_id integer CONSTRAINT case_files_case_id_fkey REFERENCES cases (id)
);
