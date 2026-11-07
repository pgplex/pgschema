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
-- Called by item_count, with a parameter renamed in new.sql: dropped and
-- created again, which must happen before item_count comes back.
CREATE FUNCTION item_scale(factor integer)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT factor::bigint;
$$;

CREATE VIEW item_count AS
SELECT count(*) * item_scale(1) AS n FROM item_view_all(now());

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

-- item_count is recreated too, so a function typed on it pins it the same
-- way, and a view calling that function binds to it: a second level of the
-- same chain, dropped before item_count and created again after it.
CREATE FUNCTION item_count_all()
RETURNS SETOF item_count
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count;
$$;

CREATE VIEW item_count_echo AS
SELECT n FROM item_count_all();

-- Dropped by the migration but typed on item_count, so it still has to go
-- before item_count's drop rather than in the drop phase.
CREATE FUNCTION item_count_legacy()
RETURNS SETOF item_count
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count;
$$;

-- Starts reading item_count in new.sql, so its desired definition must wait
-- for item_count to be back.
CREATE VIEW item_count_report AS
SELECT 0::bigint AS n;

-- Its SQL body calls item_count_all(), which only comes back after item_count,
-- so it follows that function although its signature needs item_view alone.
CREATE FUNCTION item_view_joined()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT v.* FROM item_view v CROSS JOIN item_count_all();
$$;

-- SQL-standard bodies record the call in pg_depend, so item_view_z must be
-- dropped before item_view_a although both are only typed on item_view. The
-- catalog spells item_view_a's argument differently from the normalized IR.
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

-- Not typed on any view, but its SQL-standard body records a dependency on
-- item_view_all(), so it blocks that function's drop and must go first and
-- come back after it; a plain function body ($$) would record none.
CREATE FUNCTION count_items()
RETURNS bigint
LANGUAGE sql
STABLE
BEGIN ATOMIC
    SELECT count(*) FROM item_view_all(now());
END;

-- Dropped by the migration and typed on item_count_stale, a dropped caller:
-- it must still go before that view's drop rather than in the drop phase.
CREATE FUNCTION item_count_stale_all()
RETURNS SETOF item_count_stale
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count_stale;
$$;
