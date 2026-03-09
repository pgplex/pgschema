CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    department_id INTEGER
);

-- Using ALTER VIEW SET to add security_invoker (the pattern from the issue)
CREATE VIEW public.employee_names AS
SELECT id, name FROM public.employees;

ALTER VIEW public.employee_names SET (security_invoker = true);
