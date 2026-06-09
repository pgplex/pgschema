CREATE INDEX IF NOT EXISTS events_payload_idx ON events USING gin (payload);

CREATE INDEX CONCURRENTLY IF NOT EXISTS events_2026_04_payload_idx ON events_2026_04 USING gin (payload);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'events_2026_04_payload_idx';
