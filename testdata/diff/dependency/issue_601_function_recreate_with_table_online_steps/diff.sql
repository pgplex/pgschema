CREATE INDEX IF NOT EXISTS notes_body_idx ON notes (body);

ALTER TABLE tasks ALTER COLUMN weight SET NOT NULL;

DROP FUNCTION IF EXISTS default_priority();

CREATE OR REPLACE FUNCTION default_priority()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 5
$$;

ALTER TABLE tasks ALTER COLUMN priority SET DEFAULT default_priority();

ALTER TABLE tasks ALTER COLUMN weight SET DEFAULT default_priority();
