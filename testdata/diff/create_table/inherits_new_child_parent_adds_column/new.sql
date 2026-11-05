-- The parent gains priority in the same migration that creates the child,
-- whose override of priority must wait for the parent's ADD COLUMN.
CREATE TABLE events (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
