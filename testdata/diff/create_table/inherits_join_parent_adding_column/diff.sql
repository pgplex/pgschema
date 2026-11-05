ALTER TABLE events ADD COLUMN priority integer DEFAULT 1;

ALTER TABLE alerts INHERIT events;
