CREATE INDEX IF NOT EXISTS events_payload_idx ON events USING gin (payload);
CREATE INDEX IF NOT EXISTS events_2026_04_payload_idx ON events_2026_04 USING gin (payload);
