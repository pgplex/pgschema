CREATE TABLE items (
    id bigint PRIMARY KEY,
    due date
);

CREATE VIEW item_view AS
SELECT id, due
FROM items;

CREATE FUNCTION item_view_all()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
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
