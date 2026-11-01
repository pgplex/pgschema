CREATE INDEX CONCURRENTLY IF NOT EXISTS "iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii-95791dbc_pgschema_new" ON entries (b);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii-95791dbc_pgschema_new'
  AND n.nspname = 'public';

DROP INDEX IF EXISTS iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii;

ALTER INDEX "iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii-95791dbc_pgschema_new" RENAME TO iiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiiii;
