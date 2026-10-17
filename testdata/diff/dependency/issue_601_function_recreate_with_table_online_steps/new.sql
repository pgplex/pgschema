-- Function whose return type changes (DROP + CREATE)
CREATE FUNCTION default_priority() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

-- Columns whose new default calls the function: a default-only change on a
-- NOT NULL column, and a column that also becomes NOT NULL (an online
-- rewrite with a VALIDATE of its own). Another table gains an index built
-- CONCURRENTLY. The defaults must change only inside the function's
-- transaction, never before those online steps commit.
CREATE TABLE tasks (
    id integer PRIMARY KEY,
    priority integer NOT NULL DEFAULT default_priority(),
    weight integer NOT NULL DEFAULT default_priority()
);

CREATE TABLE notes (
    id integer PRIMARY KEY,
    body text
);
CREATE INDEX notes_body_idx ON notes (body);
