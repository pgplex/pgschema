CREATE TABLE events (
    id integer NOT NULL
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);
