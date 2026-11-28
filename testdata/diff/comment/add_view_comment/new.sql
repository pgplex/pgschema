CREATE TABLE public.employees (
    id integer NOT NULL,
    name text NOT NULL,
    department text,
    salary numeric,
    active boolean DEFAULT true
);

CREATE VIEW public.employee_view AS
SELECT 
    e.id,
    e.name,
    e.department,
    e.salary
FROM public.employees e
WHERE e.active = true;

COMMENT ON VIEW public.employee_view IS 'Shows all active employees';

COMMENT ON COLUMN public.employee_view.name IS 'Employee full name';

CREATE MATERIALIZED VIEW public.department_summary AS
SELECT department, count(*) AS headcount
FROM public.employees
GROUP BY department;

COMMENT ON COLUMN public.department_summary.headcount IS 'Number of employees';

CREATE MATERIALIZED VIEW public.salary_stats AS
SELECT department, avg(salary) AS avg_salary
FROM public.employees
GROUP BY department;

COMMENT ON COLUMN public.salary_stats.avg_salary IS 'Average salary per department';
