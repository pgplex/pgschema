DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'api_role') THEN
        CREATE ROLE api_role;
    END IF;
END $$;

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
-- view, and its privileges are re-issued (issue #687). The parameter type is
-- spelled differently by the privilege catalog and the normalized IR.
CREATE FUNCTION item_view_all(p_since timestamp with time zone)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
$$;

REVOKE EXECUTE ON FUNCTION item_view_all(timestamp with time zone) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION item_view_all(timestamp with time zone) TO api_role;

-- Unchanged dependent view, rebuilt together with item_view, and an unchanged
-- function typed on it, which gets the same treatment (issue #687).
CREATE VIEW item_ids AS
SELECT id FROM item_view;

CREATE FUNCTION item_ids_all()
RETURNS SETOF item_ids
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_ids;
$$;
