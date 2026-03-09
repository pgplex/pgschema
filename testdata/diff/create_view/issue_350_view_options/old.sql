CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    email VARCHAR(100),
    department_id INTEGER
);

-- View without options (will gain security_invoker via WITH)
CREATE VIEW public.employee_names AS
SELECT id, name FROM public.employees;

-- View without options (will gain security_invoker via ALTER VIEW SET)
CREATE VIEW public.employee_emails AS
SELECT id, email FROM public.employees;

-- View with security_invoker (will be removed)
CREATE VIEW public.employee_secure WITH (security_invoker = true) AS
SELECT id, name FROM public.employees WHERE id > 0;

-- Materialized view without options (will gain fillfactor)
CREATE MATERIALIZED VIEW public.employee_summary AS
SELECT department_id, COUNT(*) AS cnt FROM public.employees GROUP BY department_id;
