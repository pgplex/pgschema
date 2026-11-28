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

COMMENT ON COLUMN public.employee_view.department IS 'Department name';

CREATE MATERIALIZED VIEW public.department_summary AS
SELECT department, count(*) AS headcount
FROM public.employees
GROUP BY department;
