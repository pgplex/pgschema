CREATE TABLE events (
    id integer NOT NULL
);

-- A standalone table that already has the column the parent is about to add
CREATE TABLE alerts (
    id integer NOT NULL,
    priority integer DEFAULT 2,
    channel text
);
