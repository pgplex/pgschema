-- The parent gains priority; the existing child's override of it follows
-- the parent's ADD COLUMN.
CREATE TABLE events (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
ALTER TABLE alerts ALTER COLUMN priority SET NOT NULL;
