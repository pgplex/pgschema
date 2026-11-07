CREATE OR REPLACE FUNCTION latest_request()
RETURNS integer
LANGUAGE plpgsql
STABLE
AS $$
BEGIN
    RETURN (SELECT max(entry.request_id) FROM entry);
END;
$$;
ALTER TABLE entry ADD COLUMN request_id integer;
ALTER TABLE entry ADD COLUMN "Priority" integer;
CREATE OR REPLACE FUNCTION add_open(
    state bigint,
    x bigint
)
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT state + x + (SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL);
$$;
CREATE OR REPLACE FUNCTION entry_rows()
RETURNS TABLE(id integer, request_id integer, "Priority" integer)
LANGUAGE sql
STABLE
AS $$
    SELECT * FROM entry;
$$;
CREATE OR REPLACE FUNCTION high_priority()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT count(*) FROM entry WHERE entry."Priority" > 1;
$$;
CREATE OR REPLACE FUNCTION open_requests()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL;
$$;
CREATE OR REPLACE FUNCTION has_open_requests()
RETURNS boolean
LANGUAGE sql
STABLE
AS $$
    SELECT open_requests() > 0;
$$;
CREATE AGGREGATE sum_open(bigint) (
    SFUNC = add_open,
    STYPE = bigint,
    INITCOND = '0'
);
CREATE OR REPLACE FUNCTION add_total(
    state bigint,
    x bigint
)
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT state + (SELECT sum_open(x));
$$;
CREATE AGGREGATE sum_total(bigint) (
    SFUNC = add_total,
    STYPE = bigint,
    INITCOND = '0'
);
CREATE OR REPLACE VIEW open_summary AS
 SELECT open_requests() AS n;
CREATE OR REPLACE VIEW total_summary AS
 SELECT sum_total(id::bigint) AS t
   FROM entry;
CREATE OR REPLACE FUNCTION add_summary(
    state bigint,
    s open_summary
)
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT state + s.n + (SELECT count(*) FROM entry WHERE entry.request_id IS NULL);
$$;
CREATE OR REPLACE FUNCTION summary_plus()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT n + (SELECT count(*) FROM entry WHERE entry.request_id IS NULL) FROM open_summary;
$$;
CREATE AGGREGATE sum_summary(open_summary) (
    SFUNC = add_summary,
    STYPE = bigint,
    INITCOND = '0'
);
CREATE OR REPLACE FUNCTION summary_total()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT sum_summary(s) FROM open_summary s;
$$;
CREATE OR REPLACE FUNCTION add_summary_total(
    state bigint,
    x bigint
)
RETURNS bigint
LANGUAGE sql
STABLE
AS $$
    SELECT state + x + summary_total();
$$;
CREATE AGGREGATE sum_summary_total(bigint) (
    SFUNC = add_summary_total,
    STYPE = bigint,
    INITCOND = '0'
);
CREATE OR REPLACE VIEW request_summary AS
 SELECT summary_plus() AS n,
    request_id
   FROM entry;
CREATE OR REPLACE VIEW request_summary_twice AS
 SELECT n * 2 AS n
   FROM request_summary;
