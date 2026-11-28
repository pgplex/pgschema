CREATE TABLE orders (
    id bigserial PRIMARY KEY,
    total numeric(12,2) NOT NULL,
    note varchar(100)
);

CREATE TABLE customers (
    id bigint PRIMARY KEY,
    name text
);

CREATE VIEW order_totals AS SELECT sum(total) AS total FROM orders;

COMMENT ON VIEW order_totals IS 'sum of all orders';

GRANT SELECT ON order_totals TO PUBLIC;

CREATE VIEW order_notes AS SELECT o.id, o.note, c.name FROM orders o JOIN customers c ON c.id = o.id;

CREATE VIEW order_notes_upper AS SELECT id, upper(note) AS note FROM order_notes;

CREATE MATERIALIZED VIEW order_totals_mv AS SELECT sum(total) AS total FROM orders;
