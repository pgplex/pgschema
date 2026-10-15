ALTER TABLE entry ADD COLUMN request_id integer;

CREATE OR REPLACE FUNCTION open_requests()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$ SELECT count(*) FROM entry WHERE entry.request_id IS NOT NULL;
$$;

CREATE OR REPLACE FUNCTION has_open_requests()
RETURNS boolean
LANGUAGE sql
STABLE
AS $$ SELECT open_requests() > 0;
$$;

CREATE OR REPLACE VIEW request_stats AS
 SELECT open_requests() AS open_count;
