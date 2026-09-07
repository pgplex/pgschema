-- Issue #580: objects that depend on a view existing at creation time.

CREATE TABLE t (
    id integer PRIMARY KEY,
    name text
);

CREATE VIEW v AS
SELECT id, name FROM t;

-- SQL-language function whose body queries the view. PostgreSQL validates a
-- SQL body at creation, so this must be created after the view.
CREATE FUNCTION count_v()
RETURNS bigint
LANGUAGE sql
AS $$ SELECT count(*) FROM v $$;

-- plpgsql trigger function whose body mentions the view. The body is not
-- validated against relations at creation, so it must NOT be deferred past
-- the trigger that references it (triggers are created before views).
CREATE FUNCTION touch_t()
RETURNS trigger
LANGUAGE plpgsql
AS $$ BEGIN PERFORM 1 FROM v; RETURN NEW; END $$;

CREATE TRIGGER touch_trg
BEFORE INSERT ON t
FOR EACH ROW EXECUTE FUNCTION touch_t();

-- Aggregate whose input type is the view's row type. Its transition function
-- takes the row type too, so the aggregate must follow both the view and the
-- view-dependent function.
CREATE FUNCTION v_sfunc(state integer, r v)
RETURNS integer
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + r.id $$;

CREATE AGGREGATE sum_v(v) (
    SFUNC = v_sfunc,
    STYPE = integer,
    INITCOND = '0'
);

-- Aggregate with ordinary types whose transition function queries the view.
-- The function is view-dependent, so the aggregate must follow it.
CREATE FUNCTION acc_with_v(state bigint, x integer)
RETURNS bigint
LANGUAGE sql
AS $$ SELECT state + x + (SELECT count(*) FROM v) $$;

CREATE AGGREGATE sum_with_v(integer) (
    SFUNC = acc_with_v,
    STYPE = bigint,
    INITCOND = '0'
);

-- View that calls the aggregate over v's row type, and a function returning
-- that view's row type. Both must follow the aggregate.
CREATE VIEW v_total AS
SELECT sum_v(v.*) AS total FROM v;

CREATE FUNCTION get_total()
RETURNS SETOF v_total
LANGUAGE sql
AS $$ SELECT * FROM v_total $$;

-- A quoted view name in a SQL-language body must still count as a dependency.
CREATE VIEW "My View" AS
SELECT id FROM t;

CREATE FUNCTION count_my_view()
RETURNS bigint
LANGUAGE sql
AS $$ SELECT count(*) FROM "My View" $$;

-- Named argument and ordered-set forms of a view row-type argument. Identity
-- args read "r v" and "ORDER BY v", and both must still count as view deps.
CREATE AGGREGATE sum_v_named(r v) (
    SFUNC = v_sfunc,
    STYPE = integer,
    INITCOND = '0'
);

CREATE AGGREGATE count_ordered_v(ORDER BY v) (
    SFUNC = v_sfunc,
    STYPE = integer,
    INITCOND = '0'
);

-- FROM ONLY is valid syntax and must still be detected as a view reference.
CREATE FUNCTION count_only_v()
RETURNS bigint
LANGUAGE sql
AS $$ SELECT count(*) FROM ONLY v $$;
