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

CREATE VIEW item_view AS
SELECT id, due
FROM items;

CREATE FUNCTION item_view_all(p_since timestamp with time zone)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
$$;

REVOKE EXECUTE ON FUNCTION item_view_all(timestamp with time zone) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION item_view_all(timestamp with time zone) TO api_role;

CREATE VIEW item_ids AS
SELECT id FROM item_view;

CREATE FUNCTION item_ids_all()
RETURNS SETOF item_ids
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_ids;
$$;

-- New and typed on the view's row type, so it must be created after the view
-- is recreated with the new column type (issue #687).
CREATE FUNCTION items_due_between(p_start date, p_end date)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT id, due FROM items WHERE due BETWEEN p_start AND p_end;
$$;

-- Revoked on a function that is only created after the view recreation, so
-- the revoke must follow that creation (issue #687).
REVOKE EXECUTE ON FUNCTION items_due_between(date, date) FROM PUBLIC;
