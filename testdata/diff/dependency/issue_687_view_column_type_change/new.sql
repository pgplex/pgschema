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

CREATE FUNCTION item_scale(f integer)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT f::bigint;
$$;

CREATE VIEW item_count AS
SELECT count(*) * item_scale(1) AS n FROM item_view_all(now());

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

CREATE FUNCTION item_count_all()
RETURNS SETOF item_count
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count;
$$;

CREATE VIEW item_count_echo AS
SELECT n FROM item_count_all();

-- New and built on item_count, which only comes back after the routines it
-- calls, so this view must be created after it rather than in the create
-- phase.
CREATE VIEW item_count_copy AS
SELECT * FROM item_count;

CREATE VIEW item_count_report AS
SELECT n FROM item_count;

CREATE FUNCTION item_view_joined()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT v.* FROM item_view v CROSS JOIN item_count_all();
$$;

CREATE FUNCTION item_view_a(p_since timestamp with time zone)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
BEGIN ATOMIC
    SELECT * FROM item_view;
END;

CREATE FUNCTION item_view_z()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
BEGIN ATOMIC
    SELECT * FROM item_view_a(now());
END;

CREATE FUNCTION count_items()
RETURNS bigint
LANGUAGE sql
STABLE
BEGIN ATOMIC
    SELECT count(*) FROM item_view_all(now());
END;

-- New, and its body calls item_view_all(), which only comes back after
-- item_view, so it is created after that rather than in the create phase.
CREATE FUNCTION count_new_items()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
SELECT count(*) FROM item_view_all(now());
$$;

-- New and typed on item_count_copy, a new view that itself waits for
-- item_count, so it waits for that view too.
CREATE FUNCTION item_count_copy_all()
RETURNS SETOF item_count_copy
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count_copy;
$$;
