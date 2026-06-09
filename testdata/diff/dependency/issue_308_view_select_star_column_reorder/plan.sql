ALTER TABLE item ADD COLUMN new_col text;

DROP MATERIALIZED VIEW IF EXISTS item_summary RESTRICT;

DROP VIEW IF EXISTS item_extended RESTRICT;

CREATE OR REPLACE VIEW item_extended AS
 SELECT i.id,
    i.title,
    i.status,
    i.new_col,
    c.name AS category_name
   FROM item i
     JOIN category c ON c.id = i.id;

CREATE MATERIALIZED VIEW IF NOT EXISTS item_summary AS
 SELECT id,
    title
   FROM item_extended;

CREATE INDEX CONCURRENTLY IF NOT EXISTS item_summary_id_idx ON item_summary (id);

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
WHERE c.relname = 'item_summary_id_idx';
