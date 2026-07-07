CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_user_summary_email_pgschema_new ON user_summary (email, status);

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
WHERE c.relname = 'idx_user_summary_email_pgschema_new';

DROP INDEX IF EXISTS idx_user_summary_email;

ALTER INDEX idx_user_summary_email_pgschema_new RENAME TO idx_user_summary_email;
