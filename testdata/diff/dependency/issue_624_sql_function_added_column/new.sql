CREATE TABLE entry (
    id int PRIMARY KEY,
    request_id int
);

-- A SQL-language function body is validated at CREATE, so this function must
-- be created after ALTER TABLE entry ADD COLUMN request_id (#624).
CREATE FUNCTION open_requests()
    RETURNS bigint
    LANGUAGE sql STABLE
    AS $$ SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL; $$;

-- A SQL caller of the deferred function must move with it.
CREATE FUNCTION has_open_requests()
    RETURNS boolean
    LANGUAGE sql STABLE
    AS $$ SELECT open_requests() > 0; $$;

-- A view calling the deferred function must follow it too.
CREATE VIEW request_stats AS
SELECT open_requests() AS open_count;
