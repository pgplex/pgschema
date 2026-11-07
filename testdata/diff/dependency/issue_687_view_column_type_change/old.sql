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

-- Views that call item_view_all() bind to the function, so they block its
-- forced drop (SQLSTATE 2BP01). They are dropped first, deepest dependent
-- first, and created again once the function is back: item_count is
-- unchanged, item_count_pairs is built on it, and item_count_stale is dropped
-- by the migration. The grant and comment on item_count are re-issued.
CREATE VIEW item_count AS
SELECT count(*) AS n FROM item_view_all(now());

COMMENT ON VIEW item_count IS 'Number of items';
GRANT SELECT ON item_count TO api_role;

CREATE VIEW item_count_pairs AS
SELECT n * 2 AS pairs FROM item_count;

CREATE VIEW item_count_stale AS
SELECT count(*) AS n FROM item_view_all(now() - interval '1 day');

-- Aggregates over item_view rows: their transition functions take the view's
-- row type, so function and aggregate both pin the view. item_id_max is kept
-- and item_id_min is dropped; both transition functions stay.
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

CREATE AGGREGATE item_id_min(item_view) (
    SFUNC = item_id_min_sfunc,
    STYPE = bigint
);

-- Reads item_view and calls the kept aggregate, so it is a dependent of the
-- recreated view that must also wait for the aggregate to be created again.
CREATE VIEW item_id_max_view AS
SELECT item_id_max(v) AS max_id FROM item_view v;
