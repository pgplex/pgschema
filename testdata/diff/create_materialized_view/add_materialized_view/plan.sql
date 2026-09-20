CREATE OR REPLACE FUNCTION employee_label(
    emp_id integer
)
RETURNS text
LANGUAGE sql
STABLE
AS $$
    SELECT e.name || ' (' || e.status || ')' FROM public.employees e WHERE e.id = emp_id
$$;

CREATE MATERIALIZED VIEW IF NOT EXISTS active_employees AS
 SELECT id,
    name,
    salary
   FROM employees
  WHERE status::text = 'active'::text;

CREATE MATERIALIZED VIEW IF NOT EXISTS employee_labels AS
 SELECT id,
    employee_label(id) AS label
   FROM employees;
