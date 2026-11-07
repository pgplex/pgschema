DROP VIEW IF EXISTS item_count_echo RESTRICT;

DROP VIEW IF EXISTS item_count_pairs RESTRICT;

DROP VIEW IF EXISTS item_count_report RESTRICT;

DROP VIEW IF EXISTS item_id_max_view RESTRICT;

DROP AGGREGATE IF EXISTS item_id_max(item_view);

DROP AGGREGATE IF EXISTS item_id_min(item_view);

DROP FUNCTION IF EXISTS count_items();

DROP FUNCTION IF EXISTS item_count_all();

DROP FUNCTION IF EXISTS item_count_legacy();

DROP VIEW IF EXISTS item_count RESTRICT;

DROP FUNCTION IF EXISTS item_count_stale_all();

DROP VIEW IF EXISTS item_count_stale RESTRICT;

DROP FUNCTION IF EXISTS item_id_max_sfunc(bigint, item_view);

DROP FUNCTION IF EXISTS item_id_min_sfunc(bigint, item_view);

DROP FUNCTION IF EXISTS item_ids_all();

DROP FUNCTION IF EXISTS item_view_all(timestamptz);

DROP FUNCTION IF EXISTS item_view_joined();

DROP FUNCTION IF EXISTS item_view_z();

DROP FUNCTION IF EXISTS item_view_a(timestamptz);

DROP FUNCTION IF EXISTS item_scale(integer);

CREATE OR REPLACE FUNCTION item_scale(
    f integer
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT f::bigint;
$$;

DROP VIEW IF EXISTS item_ids RESTRICT;

DROP VIEW IF EXISTS item_view RESTRICT;

CREATE OR REPLACE VIEW item_view AS
 SELECT id,
    due
   FROM items;

CREATE OR REPLACE VIEW item_ids AS
 SELECT id
   FROM item_view;

CREATE OR REPLACE FUNCTION item_id_max_sfunc(
    state bigint,
    item item_view
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT greatest(state, item.id);
$$;

CREATE OR REPLACE FUNCTION item_id_min_sfunc(
    state bigint,
    item item_view
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$
SELECT least(state, item.id);
$$;

CREATE OR REPLACE FUNCTION item_ids_all()
RETURNS SETOF item_ids
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_ids;
$$;

CREATE OR REPLACE FUNCTION item_view_a(
    p_since timestamptz
)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
BEGIN ATOMIC
 SELECT item_view.id,
     item_view.due
    FROM item_view;
END;

CREATE OR REPLACE FUNCTION item_view_all(
    p_since timestamptz
)
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_view;
$$;

CREATE OR REPLACE FUNCTION count_items()
RETURNS bigint
LANGUAGE sql
STABLE
BEGIN ATOMIC
 SELECT count(*) AS count
    FROM item_view_all(now()) item_view_all(id, due);
END;

CREATE OR REPLACE FUNCTION count_new_items()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
SELECT count(*) FROM item_view_all(now());
$$;

CREATE OR REPLACE FUNCTION item_view_z()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
BEGIN ATOMIC
 SELECT item_view_a.id,
     item_view_a.due
    FROM item_view_a(now()) item_view_a(id, due);
END;

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

CREATE AGGREGATE item_id_max(item_view) (
    SFUNC = item_id_max_sfunc,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE OR REPLACE VIEW item_count AS
 SELECT count(*) * item_scale(1) AS n
   FROM item_view_all(now()) item_view_all(id, due);

COMMENT ON VIEW item_count IS 'Number of items';

CREATE OR REPLACE VIEW item_count_copy AS
 SELECT n
   FROM item_count;

CREATE OR REPLACE VIEW item_count_pairs AS
 SELECT n * 2 AS pairs
   FROM item_count;

CREATE OR REPLACE VIEW item_count_report AS
 SELECT n
   FROM item_count;

CREATE OR REPLACE VIEW item_id_max_view AS
 SELECT item_id_max(v.*) AS max_id
   FROM item_view v;

CREATE OR REPLACE FUNCTION item_count_all()
RETURNS SETOF item_count
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count;
$$;

CREATE OR REPLACE FUNCTION item_count_copy_all()
RETURNS SETOF item_count_copy
LANGUAGE sql
STABLE
AS $$
SELECT * FROM item_count_copy;
$$;

CREATE OR REPLACE FUNCTION item_view_joined()
RETURNS SETOF item_view
LANGUAGE sql
STABLE
AS $$
SELECT v.* FROM item_view v CROSS JOIN item_count_all();
$$;

CREATE OR REPLACE VIEW item_count_echo AS
 SELECT n
   FROM item_count_all() item_count_all(n);

REVOKE EXECUTE ON FUNCTION item_view_all(p_since timestamp with time zone) FROM PUBLIC;

REVOKE EXECUTE ON FUNCTION items_due_between(p_start date, p_end date) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION item_view_all(p_since timestamp with time zone) TO api_role;

GRANT SELECT ON TABLE item_count TO api_role;
