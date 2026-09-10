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

DROP INDEX IF EXISTS orders_code_key;

CREATE UNIQUE INDEX IF NOT EXISTS orders_code_key ON orders (code);

DROP INDEX IF EXISTS orders_lookup_idx;

CREATE INDEX IF NOT EXISTS orders_lookup_idx ON orders (total);

ALTER TABLE shipments
ADD CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES orders (code);

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
