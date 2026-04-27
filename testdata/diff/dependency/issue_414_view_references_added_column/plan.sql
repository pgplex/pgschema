ALTER TABLE foo ADD COLUMN run_id uuid;

CREATE OR REPLACE VIEW foo_view AS
 SELECT id,
    run_id
   FROM foo
  WHERE run_id IS NOT NULL;
