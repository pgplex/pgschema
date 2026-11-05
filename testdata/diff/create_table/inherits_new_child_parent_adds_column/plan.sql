CREATE TABLE IF NOT EXISTS alerts (
    channel text
) INHERITS (events);

ALTER TABLE events ADD COLUMN priority integer DEFAULT 1;

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
