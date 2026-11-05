-- The parent adds priority; the child only comments on it.
CREATE TABLE events (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

COMMENT ON COLUMN alerts.priority IS 'Paging priority for this alert';
