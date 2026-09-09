DROP VIEW IF EXISTS big_orders RESTRICT;

DROP VIEW IF EXISTS order_totals RESTRICT;

ALTER TABLE shipments DROP CONSTRAINT shipments_order_code_fkey;

ALTER TABLE orders DROP COLUMN total;

ALTER TABLE orders DROP COLUMN code;

ALTER TABLE orders ADD COLUMN total integer GENERATED ALWAYS AS ((qty * price)) STORED;

ALTER TABLE orders ADD COLUMN code text GENERATED ALWAYS AS (('ORD-'::text || (id)::text)) STORED;

DROP INDEX IF EXISTS orders_lookup_idx;

CREATE INDEX IF NOT EXISTS orders_lookup_idx ON orders (total);

CREATE UNIQUE INDEX IF NOT EXISTS orders_code_key ON orders (code);

ALTER TABLE shipments
ADD CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES orders (code);

CREATE OR REPLACE VIEW order_totals AS
 SELECT id,
    total
   FROM orders;

CREATE OR REPLACE VIEW big_orders AS
 SELECT id
   FROM order_totals
  WHERE total > 100;

GRANT SELECT (id, total) ON TABLE orders TO app_reader;
