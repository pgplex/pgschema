CREATE UNLOGGED TABLE IF NOT EXISTS events (
    id integer,
    payload text NOT NULL,
    CONSTRAINT events_pkey PRIMARY KEY (id)
);
