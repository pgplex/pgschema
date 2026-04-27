ALTER TABLE foo ADD COLUMN run_id uuid;

CREATE OR REPLACE VIEW foo_base AS
 SELECT id,
    run_id
   FROM foo
  WHERE run_id IS NOT NULL;

CREATE OR REPLACE VIEW foo_summary AS
 SELECT id
   FROM foo_base;

CREATE OR REPLACE FUNCTION get_foo_summary()
RETURNS SETOF foo_summary
LANGUAGE sql
STABLE
AS $$ SELECT * FROM foo_summary
$$;
