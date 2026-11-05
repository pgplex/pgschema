ALTER TABLE z_parent ADD COLUMN region text;

ALTER TABLE a_child
ADD CONSTRAINT a_child_uid_key UNIQUE (uid);

CREATE INDEX CONCURRENTLY IF NOT EXISTS a_child_region_idx ON a_child (region);

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
WHERE c.relname = 'a_child_region_idx'
  AND n.nspname = 'public';

ALTER TABLE z_parent
ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid) NOT VALID;

ALTER TABLE z_parent VALIDATE CONSTRAINT z_parent_favorite_uid_fkey;
