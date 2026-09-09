ALTER TABLE metric_refs DROP CONSTRAINT metric_refs_tripled_fkey;

ALTER TABLE metrics DROP COLUMN tripled;

ALTER TABLE metrics
ADD COLUMN tripled integer GENERATED ALWAYS AS ((a * 3)) STORED CONSTRAINT metrics_tripled_key UNIQUE;

ALTER TABLE metrics ALTER COLUMN doubled SET EXPRESSION AS ((a * 2));

ALTER TABLE metrics ALTER COLUMN label DROP EXPRESSION;

ALTER TABLE metrics ALTER COLUMN label SET DEFAULT 'none';

ALTER TABLE metrics
ADD CONSTRAINT metrics_tripled_check CHECK (tripled > 0) NOT VALID;

ALTER TABLE metrics VALIDATE CONSTRAINT metrics_tripled_check;

CREATE INDEX CONCURRENTLY IF NOT EXISTS metrics_tripled_idx ON metrics ((tripled + 1)) WHERE (tripled > 10);

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
WHERE c.relname = 'metrics_tripled_idx';

ALTER TABLE metric_refs
ADD CONSTRAINT metric_refs_tripled_fkey FOREIGN KEY (tripled) REFERENCES metrics (tripled) NOT VALID;

ALTER TABLE metric_refs VALIDATE CONSTRAINT metric_refs_tripled_fkey;
