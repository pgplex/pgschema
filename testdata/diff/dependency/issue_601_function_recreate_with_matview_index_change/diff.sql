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

CREATE INDEX IF NOT EXISTS document_titles_title_idx ON document_titles (title);
