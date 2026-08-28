CREATE TABLE IF NOT EXISTS events (
    id integer,
    occurred_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT events_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_events_desc ON events (occurred_at DESC NULLS LAST);
