CREATE TABLE entry (
    id int PRIMARY KEY,
    request_id int,
    "Priority" int
);

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

-- A SELECT * body is checked against the RETURNS TABLE list at creation.
CREATE FUNCTION entry_rows() RETURNS TABLE(id int, request_id int, "Priority" int) LANGUAGE sql STABLE AS $$
    SELECT * FROM entry;
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
