CREATE TABLE IF NOT EXISTS child (
    id uuid,
    parent_id uuid,
    tenant text NOT NULL,
    CONSTRAINT child_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS child2 (
    id uuid,
    parent2_id uuid,
    tenant text NOT NULL,
    CONSTRAINT child2_pkey PRIMARY KEY (id)
);

ALTER TABLE parent
ADD CONSTRAINT parent_id_tenant_key UNIQUE (id, tenant);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS parent2_id_tenant_key ON parent2 (id, tenant);

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
WHERE c.relname = 'parent2_id_tenant_key';

ALTER TABLE child
ADD CONSTRAINT child_parent_id_tenant_fkey FOREIGN KEY (parent_id, tenant) REFERENCES parent (id, tenant);

ALTER TABLE child2
ADD CONSTRAINT child2_parent2_id_tenant_fkey FOREIGN KEY (parent2_id, tenant) REFERENCES parent2 (id, tenant);
