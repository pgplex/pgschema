ALTER TABLE item ADD COLUMN new_col text;

DROP MATERIALIZED VIEW IF EXISTS item_summary RESTRICT;

DROP VIEW IF EXISTS item_extended RESTRICT;

CREATE OR REPLACE VIEW item_extended AS
 SELECT i.id,
    i.title,
    i.status,
    i.new_col,
    c.name AS category_name
   FROM item i
     JOIN category c ON c.id = i.id;

CREATE MATERIALIZED VIEW IF NOT EXISTS item_summary AS
 SELECT id,
    title
   FROM item_extended;

CREATE INDEX IF NOT EXISTS item_summary_id_idx ON item_summary (id);
