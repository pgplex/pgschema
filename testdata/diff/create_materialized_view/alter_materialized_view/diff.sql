DROP MATERIALIZED VIEW active_employees RESTRICT;

CREATE MATERIALIZED VIEW IF NOT EXISTS active_employees AS
 SELECT id,
    name,
    salary,
    status
   FROM employees
  WHERE status::text = 'active'::text;

COMMENT ON COLUMN active_employees.name IS 'Employee name';
