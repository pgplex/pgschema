CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    department_id INTEGER
);

CREATE VIEW public.employee_names WITH (security_invoker = true) AS
SELECT id, name FROM public.employees;
