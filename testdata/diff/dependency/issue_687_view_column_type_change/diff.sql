DROP FUNCTION IF EXISTS item_view_all();

DROP VIEW IF EXISTS item_view RESTRICT;

CREATE OR REPLACE VIEW item_view AS
 SELECT id,
    due
   FROM items;

CREATE OR REPLACE FUNCTION item_view_all()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
$$;

CREATE OR REPLACE FUNCTION items_due_between(
    p_start date,
    p_end date
)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT id, due FROM items WHERE due BETWEEN p_start AND p_end;
$$;
