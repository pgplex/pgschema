ALTER TYPE ticket_state ADD VALUE 'pending' AFTER 'open';

ALTER TABLE tickets ALTER COLUMN state SET DEFAULT 'pending'::ticket_state;

ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_priority_check;

ALTER TABLE tickets ALTER COLUMN priority DROP DEFAULT;

DROP FUNCTION IF EXISTS default_priority();

CREATE OR REPLACE FUNCTION default_priority()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 5
$$;

ALTER TABLE tickets ALTER COLUMN priority SET DEFAULT default_priority();

ALTER TABLE tickets
ADD CONSTRAINT tickets_priority_check CHECK (priority <= (default_priority() * 10)) NOT VALID;

ALTER TABLE tickets VALIDATE CONSTRAINT tickets_priority_check;
