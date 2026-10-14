ALTER TABLE "order"
ADD COLUMN tenant_id uuid CONSTRAINT "FK_order_tenant" REFERENCES tenant (id);

CREATE INDEX CONCURRENTLY IF NOT EXISTS "IDX_order_tenant_order_number" ON "order" (tenant_id, order_number);

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
WHERE c.relname = 'IDX_order_tenant_order_number'
  AND c.relnamespace = 'public'::regnamespace;
