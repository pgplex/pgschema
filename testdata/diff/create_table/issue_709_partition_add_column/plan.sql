ALTER TABLE events DROP COLUMN legacy;

ALTER TABLE events ADD COLUMN payload text;

ALTER TABLE events ALTER COLUMN amount TYPE bigint USING amount::bigint;
