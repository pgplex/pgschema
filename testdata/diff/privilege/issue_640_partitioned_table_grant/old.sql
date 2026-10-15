DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'reader') THEN
        CREATE ROLE reader;
    END IF;
END $$;

CREATE TABLE events (
    id int NOT NULL,
    at date NOT NULL
) PARTITION BY RANGE (at);

CREATE TABLE events_2026_01 PARTITION OF events FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

GRANT INSERT ON events TO reader;
