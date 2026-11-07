CREATE TABLE entry (
    id int PRIMARY KEY,
    request_id int
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
