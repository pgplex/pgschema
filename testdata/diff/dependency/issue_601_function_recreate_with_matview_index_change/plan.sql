DROP POLICY IF EXISTS documents_tenant ON documents;

ALTER TABLE documents ALTER COLUMN tenant_id DROP DEFAULT;

DROP FUNCTION IF EXISTS current_tenant();

CREATE OR REPLACE FUNCTION current_tenant()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$ SELECT current_setting('app.tenant', true)::bigint
$$;

ALTER TABLE documents ALTER COLUMN tenant_id SET DEFAULT current_tenant();

CREATE POLICY documents_tenant ON documents AS RESTRICTIVE TO tenant_reader USING (tenant_id = current_tenant());

CREATE INDEX CONCURRENTLY IF NOT EXISTS document_titles_title_idx ON document_titles (title);

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
WHERE c.relname = 'document_titles_title_idx';
