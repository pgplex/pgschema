DROP VIEW IF EXISTS item_id_max_view RESTRICT;

DROP VIEW IF EXISTS item_count_stale RESTRICT;

DROP VIEW IF EXISTS item_count_pairs RESTRICT;

DROP VIEW IF EXISTS item_count RESTRICT;

DROP AGGREGATE IF EXISTS item_id_max(item_view);

DROP AGGREGATE IF EXISTS item_id_min(item_view);

DROP FUNCTION IF EXISTS item_view_all(timestamptz);

DROP FUNCTION IF EXISTS item_ids_all();

DROP FUNCTION IF EXISTS item_id_min_sfunc(bigint, item_view);

DROP FUNCTION IF EXISTS item_id_max_sfunc(bigint, item_view);

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

CREATE OR REPLACE FUNCTION item_view_all(
    p_since timestamptz
)
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

CREATE AGGREGATE item_id_max(item_view) (
    SFUNC = item_id_max_sfunc,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE OR REPLACE VIEW item_count AS
 SELECT count(*) AS n
   FROM item_view_all(now()) item_view_all(id, due);

COMMENT ON VIEW item_count IS 'Number of items';

CREATE OR REPLACE VIEW item_count_pairs AS
 SELECT n * 2 AS pairs
   FROM item_count;

CREATE OR REPLACE VIEW item_id_max_view AS
 SELECT item_id_max(v.*) AS max_id
   FROM item_view v;

REVOKE EXECUTE ON FUNCTION item_view_all(p_since timestamp with time zone) FROM PUBLIC;

REVOKE EXECUTE ON FUNCTION items_due_between(p_start date, p_end date) FROM PUBLIC;

GRANT EXECUTE ON FUNCTION item_view_all(p_since timestamp with time zone) TO api_role;

GRANT SELECT ON TABLE item_count TO api_role;
