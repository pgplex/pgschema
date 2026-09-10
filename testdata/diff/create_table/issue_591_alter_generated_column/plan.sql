DROP VIEW IF EXISTS big_orders RESTRICT;

DROP VIEW IF EXISTS order_totals RESTRICT;

DROP VIEW IF EXISTS order_labels RESTRICT;

DROP TRIGGER IF EXISTS orders_total_trg ON orders;

ALTER TABLE metric_refs DROP CONSTRAINT metric_refs_tripled_fkey;

ALTER TABLE shipments DROP CONSTRAINT shipments_order_code_fkey;

ALTER TABLE metrics DROP COLUMN tripled;

ALTER TABLE metrics
ADD COLUMN tripled integer GENERATED ALWAYS AS ((a * 3)) STORED CONSTRAINT metrics_tripled_key UNIQUE;

ALTER TABLE metrics ALTER COLUMN doubled SET EXPRESSION AS ((a * 2));

ALTER TABLE metrics ALTER COLUMN label DROP EXPRESSION;

ALTER TABLE metrics ALTER COLUMN label SET DEFAULT 'none';

ALTER TABLE metrics
ADD CONSTRAINT metrics_tripled_check CHECK (tripled > 0) NOT VALID;

ALTER TABLE metrics VALIDATE CONSTRAINT metrics_tripled_check;

CREATE INDEX CONCURRENTLY IF NOT EXISTS metrics_tripled_idx_pgschema_new ON metrics ((tripled + 1)) WHERE (tripled > 10);

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
WHERE c.relname = 'metrics_tripled_idx_pgschema_new';

DROP INDEX IF EXISTS metrics_tripled_idx;

ALTER INDEX metrics_tripled_idx_pgschema_new RENAME TO metrics_tripled_idx;

DROP POLICY IF EXISTS orders_big ON orders;

ALTER TABLE orders DROP CONSTRAINT orders_total_excl;

ALTER TABLE orders DROP COLUMN total;

ALTER TABLE orders DROP COLUMN code;

ALTER TABLE orders ADD COLUMN total integer GENERATED ALWAYS AS ((qty * price)) STORED;

ALTER TABLE orders ADD COLUMN code text GENERATED ALWAYS AS (('ORD-'::text || (id)::text)) STORED;

ALTER TABLE orders
ADD CONSTRAINT orders_total_excl EXCLUDE USING btree ((total + 0) WITH =);

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

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS orders_total_key ON orders (total);

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
WHERE c.relname = 'orders_total_key';

ALTER TABLE vt DROP COLUMN v2;

ALTER TABLE vt DROP COLUMN s1;

ALTER TABLE vt ADD COLUMN v2 integer;

ALTER TABLE vt ADD COLUMN s1 integer GENERATED ALWAYS AS ((a * 2)) VIRTUAL;

ALTER TABLE vt ALTER COLUMN v1 SET EXPRESSION AS ((a * 2));

ALTER TABLE metric_refs
ADD CONSTRAINT metric_refs_tripled_fkey FOREIGN KEY (tripled) REFERENCES metrics (tripled) NOT VALID;

ALTER TABLE metric_refs VALIDATE CONSTRAINT metric_refs_tripled_fkey;

ALTER TABLE returns
ADD CONSTRAINT returns_order_code_fkey FOREIGN KEY (order_code) REFERENCES orders (code) NOT VALID;

ALTER TABLE returns VALIDATE CONSTRAINT returns_order_code_fkey;

ALTER TABLE returns
ADD CONSTRAINT returns_order_total_fkey FOREIGN KEY (order_total) REFERENCES orders (total) NOT VALID;

ALTER TABLE returns VALIDATE CONSTRAINT returns_order_total_fkey;

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

GRANT SELECT (id) ON TABLE big_orders TO app_reader;

GRANT SELECT (id, total) ON TABLE orders TO app_reader;
