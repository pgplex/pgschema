CREATE TABLE public.employees (
    id serial PRIMARY KEY,
    name text NOT NULL,
    last_modified timestamp DEFAULT CURRENT_TIMESTAMP
);

CREATE OR REPLACE FUNCTION public.update_last_modified()
RETURNS trigger AS $$
BEGIN
    NEW.last_modified = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER employees_last_modified_trigger
    BEFORE UPDATE ON public.employees
    FOR EACH ROW
    EXECUTE FUNCTION public.update_last_modified();

COMMENT ON TRIGGER employees_last_modified_trigger ON public.employees IS 'Updates last_modified timestamp to current time on every row update';

CREATE VIEW public.employee_emails AS
SELECT id, name
FROM public.employees;

CREATE OR REPLACE FUNCTION public.insert_employee_emails()
RETURNS trigger AS $$
BEGIN
    INSERT INTO public.employees (name)
    VALUES (NEW.name)
    RETURNING id, name INTO NEW.id, NEW.name;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_employee_emails_insert
    INSTEAD OF INSERT ON public.employee_emails
    FOR EACH ROW
    EXECUTE FUNCTION public.insert_employee_emails();

COMMENT ON TRIGGER trg_employee_emails_insert ON public.employee_emails IS 'Handles inserts into the employee_emails view';
