ALTER TABLE foo ADD COLUMN run_id uuid;

CREATE OR REPLACE VIEW foo_base AS
 SELECT id,
    run_id
   FROM foo
  WHERE run_id IS NOT NULL;

CREATE OR REPLACE VIEW foo_summary AS
 SELECT id
   FROM foo_base;

CREATE OR REPLACE FUNCTION foo_summary_step(
    state bigint,
    r foo_summary
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + r.id
$$;

CREATE OR REPLACE FUNCTION get_foo_summary()
RETURNS SETOF foo_summary
LANGUAGE sql
STABLE
AS $$ SELECT * FROM foo_summary
$$;

CREATE AGGREGATE sum_foo_summary(foo_summary) (
    SFUNC = foo_summary_step,
    STYPE = bigint,
    INITCOND = '0'
);

CREATE OR REPLACE VIEW foo_total AS
 SELECT sum_foo_summary(foo_summary.*) AS total
   FROM foo_summary;
