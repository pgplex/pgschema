DROP VIEW IF EXISTS order_totals RESTRICT;

DROP VIEW IF EXISTS order_notes_upper RESTRICT;

DROP VIEW IF EXISTS order_notes RESTRICT;

DROP MATERIALIZED VIEW order_totals_mv RESTRICT;

ALTER TABLE orders ALTER COLUMN total TYPE numeric(12,2);

ALTER TABLE orders ALTER COLUMN note TYPE varchar(100);

CREATE OR REPLACE VIEW order_notes AS
 SELECT o.id,
    o.note,
    c.name
   FROM orders o
     JOIN customers c ON c.id = o.id;

CREATE OR REPLACE VIEW order_totals AS
 SELECT sum(total) AS total
   FROM orders;

COMMENT ON VIEW order_totals IS 'sum of all orders';

CREATE MATERIALIZED VIEW IF NOT EXISTS order_totals_mv AS
 SELECT sum(total) AS total
   FROM orders;

CREATE OR REPLACE VIEW order_notes_upper AS
 SELECT id,
    upper(note::text) AS note
   FROM order_notes;

GRANT SELECT ON TABLE order_totals TO PUBLIC;
