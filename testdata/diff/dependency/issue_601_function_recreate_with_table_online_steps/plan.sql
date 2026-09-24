CREATE INDEX CONCURRENTLY IF NOT EXISTS notes_body_idx ON notes (body);

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
WHERE c.relname = 'notes_body_idx';

ALTER TABLE tasks ADD CONSTRAINT tasks_weight_not_null NOT NULL weight NOT VALID;

ALTER TABLE tasks VALIDATE CONSTRAINT tasks_weight_not_null;

DROP FUNCTION IF EXISTS default_priority();

CREATE OR REPLACE FUNCTION default_priority()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 5
$$;

ALTER TABLE tasks ALTER COLUMN priority SET DEFAULT default_priority();

ALTER TABLE tasks ALTER COLUMN weight SET DEFAULT default_priority();
