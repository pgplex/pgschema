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

CREATE VIEW item_count AS
SELECT count(*) AS n FROM item_view_all(now());

COMMENT ON VIEW item_count IS 'Number of items';
GRANT SELECT ON item_count TO api_role;

CREATE VIEW item_count_pairs AS
SELECT n * 2 AS pairs FROM item_count;

CREATE FUNCTION item_id_max_sfunc(state bigint, item item_view)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT greatest(state, item.id);
$$;

CREATE AGGREGATE item_id_max(item_view) (
    SFUNC = item_id_max_sfunc,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE FUNCTION item_id_min_sfunc(state bigint, item item_view)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT least(state, item.id);
$$;

CREATE VIEW item_id_max_view AS
SELECT item_id_max(v) AS max_id FROM item_view v;
