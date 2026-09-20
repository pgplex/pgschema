ALTER TYPE status ADD VALUE 'archived' AFTER 'pending';

ALTER TABLE work ALTER COLUMN state SET DEFAULT 'archived'::status;
