DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tenant_reader') THEN
        CREATE ROLE tenant_reader;
    END IF;
END $$;

-- Function whose return type changes (DROP + CREATE)
CREATE FUNCTION current_tenant() RETURNS bigint LANGUAGE sql STABLE AS $$ SELECT current_setting('app.tenant', true)::bigint $$;

-- Policy and default held around the recreation
CREATE TABLE documents (
    id integer PRIMARY KEY,
    tenant_id integer NOT NULL DEFAULT current_tenant(),
    title text
);
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
CREATE POLICY documents_tenant ON documents AS RESTRICTIVE TO tenant_reader USING (tenant_id = current_tenant());
CREATE POLICY documents_read ON documents TO tenant_reader USING (true);

-- Unrelated materialized view that gains an index (built CONCURRENTLY in its
-- own transaction): it must not split the drops from the restores
CREATE MATERIALIZED VIEW document_titles AS SELECT id, title FROM documents;
CREATE INDEX document_titles_title_idx ON document_titles (title);
