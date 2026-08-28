DROP INDEX IF EXISTS idx_events_desc;

CREATE INDEX IF NOT EXISTS idx_events_desc ON events (occurred_at DESC NULLS LAST);
