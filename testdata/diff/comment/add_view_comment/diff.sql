CREATE MATERIALIZED VIEW IF NOT EXISTS salary_stats AS
 SELECT department,
    avg(salary) AS avg_salary
   FROM employees
  GROUP BY department;

COMMENT ON COLUMN salary_stats.avg_salary IS 'Average salary per department';

COMMENT ON COLUMN department_summary.headcount IS 'Number of employees';

COMMENT ON VIEW employee_view IS 'Shows all active employees';

COMMENT ON COLUMN employee_view.name IS 'Employee full name';

COMMENT ON COLUMN employee_view.department IS NULL;
