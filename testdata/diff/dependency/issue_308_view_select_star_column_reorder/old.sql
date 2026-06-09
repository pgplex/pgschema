CREATE TABLE item (
    id uuid PRIMARY KEY,
    title text,
    status text
);

CREATE TABLE category (
    id uuid PRIMARY KEY,
    name text
);

CREATE VIEW item_extended AS
SELECT i.*, c.name AS category_name
FROM item i
JOIN category c ON c.id = i.id;

-- Materialized view depending on the recreated view (issue #415):
-- when item_extended is recreated, this MV must be dropped with
-- DROP MATERIALIZED VIEW, not DROP VIEW.
CREATE MATERIALIZED VIEW item_summary AS
SELECT id, title FROM item_extended;
