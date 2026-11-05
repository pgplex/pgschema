ALTER TABLE z_root ADD COLUMN region text;

CREATE INDEX CONCURRENTLY IF NOT EXISTS a_leaf_region_idx ON a_leaf (region);

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
WHERE c.relname = 'a_leaf_region_idx'
  AND n.nspname = 'public';
