ALTER TABLE tickets DROP COLUMN priority;

ALTER TABLE tickets ALTER COLUMN severity TYPE text USING severity::text;

DROP TYPE IF EXISTS severity RESTRICT;

DROP TYPE IF EXISTS priority RESTRICT;
