CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100)
);

-- View with security_invoker via CREATE VIEW WITH
CREATE VIEW public.employee_names WITH (security_invoker = true) AS
SELECT id, name FROM public.employees;

-- View with security_invoker via ALTER VIEW SET
CREATE VIEW public.employee_emails AS
SELECT id, email FROM public.employees;

ALTER VIEW public.employee_emails SET (security_invoker = true);

-- View with security_barrier
CREATE VIEW public.employee_secure WITH (security_barrier = true) AS
SELECT id, name FROM public.employees WHERE id > 0;
