ALTER TABLE notifications DROP COLUMN valid_period;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_notifications_tenant_user_pgschema_new ON notifications (tenant_id, user_id);

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
WHERE c.relname = 'idx_notifications_tenant_user_pgschema_new';

DROP INDEX IF EXISTS idx_notifications_tenant_user;

ALTER INDEX idx_notifications_tenant_user_pgschema_new RENAME TO idx_notifications_tenant_user;
