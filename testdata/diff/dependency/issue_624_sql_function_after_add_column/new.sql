CREATE TABLE entry (
    id int PRIMARY KEY,
    request_id int,
    "Priority" int
);
CREATE TABLE entry_child () INHERITS (entry);

-- Issue #624: a SQL-language body is validated at creation, so this function
-- must be created after ALTER TABLE entry ADD COLUMN request_id.
CREATE FUNCTION open_requests() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL;
$$;

-- A SQL function calling open_requests() is validated against it too, so it
-- waits with it even though it never reads the column itself.
CREATE FUNCTION has_open_requests() RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT open_requests() > 0;
$$;

-- A plpgsql body resolves columns at run time, so it is created as usual.
CREATE FUNCTION latest_request() RETURNS int LANGUAGE plpgsql STABLE AS $$
BEGIN
    RETURN (SELECT max(entry.request_id) FROM entry);
END;
$$;

-- A column that needs quoting is matched in its exact quoted spelling.
CREATE FUNCTION high_priority() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT count(*) FROM entry WHERE entry."Priority" > 1;
$$;

-- A SELECT * body is checked against the RETURNS TABLE list at creation,
-- positionally, whatever the output columns are called.
CREATE FUNCTION entry_rows() RETURNS TABLE(id int, request_id int, "Priority" int) LANGUAGE sql STABLE AS $$
    SELECT * FROM entry;
$$;
CREATE FUNCTION entry_triples() RETURNS TABLE(a int, b int, c int) LANGUAGE sql STABLE AS $$
    SELECT e.* FROM entry e;
$$;

-- The column reaches the inheritance child through the parent's ALTER TABLE,
-- so a reader of the child waits for it too.
CREATE FUNCTION child_open_requests() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT count(*) FROM entry_child WHERE entry_child.request_id IS NOT NULL;
$$;

-- Aggregate chain: add_open reads the column, sum_open is built on it,
-- add_total calls sum_open, sum_total is built on add_total. All four wait
-- for the column.
CREATE FUNCTION add_open(state bigint, x bigint) RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT state + x + (SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL);
$$;
CREATE AGGREGATE sum_open(bigint) (SFUNC = add_open, STYPE = bigint, INITCOND = '0');
CREATE FUNCTION add_total(state bigint, x bigint) RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT state + (SELECT sum_open(x));
$$;
CREATE AGGREGATE sum_total(bigint) (SFUNC = add_total, STYPE = bigint, INITCOND = '0');

-- A new view calling a held function or aggregate waits for it, and a held
-- SQL function reading that view waits for the view in turn.
CREATE VIEW open_summary AS SELECT open_requests() AS n;
CREATE VIEW total_summary AS SELECT sum_total(id::bigint) AS t FROM entry;
CREATE FUNCTION summary_plus() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT n + (SELECT count(*) FROM entry WHERE entry.request_id IS NULL) FROM open_summary;
$$;

-- A view that reads the added column (held like #414) and calls a function
-- moved to the later batch moves there too, with the views built on it.
CREATE VIEW request_summary AS SELECT summary_plus() AS n, request_id FROM entry;
CREATE VIEW request_summary_twice AS SELECT n * 2 AS n FROM request_summary;

-- A chain crossing the aggregate pools: sum_summary takes a new view's row
-- type and is built on a column reader, summary_total calls it, and
-- sum_summary_total is built on summary_total.
CREATE FUNCTION add_summary(state bigint, s open_summary) RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT state + s.n + (SELECT count(*) FROM entry WHERE entry.request_id IS NULL);
$$;
CREATE AGGREGATE sum_summary(open_summary) (SFUNC = add_summary, STYPE = bigint, INITCOND = '0');
CREATE FUNCTION summary_total() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT sum_summary(s) FROM open_summary s;
$$;
CREATE FUNCTION add_summary_total(state bigint, x bigint) RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT state + x + summary_total();
$$;
CREATE AGGREGATE sum_summary_total(bigint) (SFUNC = add_summary_total, STYPE = bigint, INITCOND = '0');
