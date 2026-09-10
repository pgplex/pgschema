DROP VIEW IF EXISTS big_orders RESTRICT;

DROP VIEW IF EXISTS order_totals RESTRICT;

DROP VIEW IF EXISTS order_labels RESTRICT;

DROP TRIGGER IF EXISTS orders_total_trg ON orders;

ALTER TABLE shipments DROP CONSTRAINT shipments_order_code_fkey;

DROP POLICY IF EXISTS orders_big ON orders;

ALTER TABLE orders DROP COLUMN total;

ALTER TABLE orders DROP COLUMN code;

ALTER TABLE orders ADD COLUMN total integer GENERATED ALWAYS AS ((qty * price)) STORED;

ALTER TABLE orders ADD COLUMN code text GENERATED ALWAYS AS (('ORD-'::text || (id)::text)) STORED;

CREATE OR REPLACE TRIGGER orders_total_trg
    AFTER UPDATE ON orders
    FOR EACH ROW
    WHEN (((NEW.total > 0)))
    EXECUTE FUNCTION orders_audit();

CREATE POLICY orders_big ON orders TO PUBLIC USING (total > 100);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS orders_code_key_pgschema_new ON orders (code);

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
WHERE c.relname = 'orders_code_key_pgschema_new';

DROP INDEX IF EXISTS orders_code_key;

ALTER INDEX orders_code_key_pgschema_new RENAME TO orders_code_key;

CREATE INDEX CONCURRENTLY IF NOT EXISTS orders_lookup_idx_pgschema_new ON orders (total);

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
WHERE c.relname = 'orders_lookup_idx_pgschema_new';

DROP INDEX IF EXISTS orders_lookup_idx;

ALTER INDEX orders_lookup_idx_pgschema_new RENAME TO orders_lookup_idx;

ALTER TABLE shipments
ADD CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES orders (code) NOT VALID;

ALTER TABLE shipments VALIDATE CONSTRAINT shipments_order_code_fkey;

CREATE OR REPLACE VIEW order_labels AS
 SELECT id,
    'x'::text AS label
   FROM orders;

CREATE OR REPLACE VIEW order_totals AS
 SELECT id,
    total
   FROM orders;

CREATE OR REPLACE VIEW big_orders AS
 SELECT id
   FROM order_totals
  WHERE total > 100;

GRANT SELECT ON TABLE order_totals TO app_reader;

GRANT SELECT (id, total) ON TABLE orders TO app_reader;
