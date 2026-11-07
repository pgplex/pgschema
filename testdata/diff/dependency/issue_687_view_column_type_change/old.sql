CREATE TABLE items (
    id bigint PRIMARY KEY,
    due date
);

-- The view's "due" column is text here and becomes date in new.sql. CREATE OR
-- REPLACE VIEW rejects a column type change (SQLSTATE 42P16), so the view must
-- be dropped and created again (issue #687).
CREATE VIEW item_view AS
SELECT id, to_char(due, 'MM/DD/YYYY') AS due
FROM items;

-- Unchanged in new.sql. Typed on the view's row type, it would block the
-- view's RESTRICT drop, so it is dropped first and created again after the
-- view (issue #687).
CREATE FUNCTION item_view_all()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
$$;
