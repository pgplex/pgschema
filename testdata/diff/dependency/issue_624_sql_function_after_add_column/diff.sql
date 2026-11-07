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
