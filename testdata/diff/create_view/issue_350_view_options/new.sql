CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    department_id INTEGER
);

-- Add security_invoker via CREATE VIEW WITH
CREATE VIEW public.employee_names WITH (security_invoker = true) AS
SELECT id, name FROM public.employees;

-- Add security_invoker via ALTER VIEW SET
CREATE VIEW public.employee_emails AS
SELECT id, email FROM public.employees;

ALTER VIEW public.employee_emails SET (security_invoker = true);

-- Remove security_invoker
CREATE VIEW public.employee_secure AS
SELECT id, name FROM public.employees WHERE id > 0;
