CREATE TABLE public.employees (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    salary DECIMAL(10,2) NOT NULL,
    status VARCHAR(20) NOT NULL
);

CREATE MATERIALIZED VIEW public.active_employees AS
SELECT 
    id,
    name,
    salary
FROM employees
WHERE status = 'active';

-- Issue #596: a SQL function whose body qualifies a table with the managed
-- schema is inlined while the materialized view's query is planned.
CREATE FUNCTION public.employee_label(emp_id integer) RETURNS text
LANGUAGE sql STABLE
AS $$
    SELECT e.name || ' (' || e.status || ')' FROM public.employees e WHERE e.id = emp_id
$$;

CREATE MATERIALIZED VIEW public.employee_labels AS
SELECT
    id,
    employee_label(id) AS label
FROM employees;
