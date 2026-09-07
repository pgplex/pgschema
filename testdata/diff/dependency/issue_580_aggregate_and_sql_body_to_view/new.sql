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
