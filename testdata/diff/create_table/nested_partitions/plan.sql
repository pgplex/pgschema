CREATE TABLE IF NOT EXISTS events (
    bucket integer NOT NULL,
    label text
) PARTITION BY RANGE (bucket);

CREATE TABLE IF NOT EXISTS events_early PARTITION OF events (
    label DEFAULT 'pending'
) FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (bucket);

CREATE TABLE IF NOT EXISTS events_first PARTITION OF events_early FOR VALUES FROM (0) TO (5);

CREATE TABLE IF NOT EXISTS events_second PARTITION OF events_early FOR VALUES FROM (5) TO (10);
