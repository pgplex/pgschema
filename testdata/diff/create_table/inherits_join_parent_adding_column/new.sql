-- alerts joins events in the same migration that adds priority to events;
-- alerts brings its own priority (with its own default) to the INHERIT.
CREATE TABLE events (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
