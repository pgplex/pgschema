CREATE TABLE public.employees (
    id serial PRIMARY KEY,
    name text NOT NULL,
    salary numeric(10,2),
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
    BEFORE INSERT OR UPDATE OF salary ON public.employees
    FOR EACH ROW
    WHEN (NEW.salary IS NOT NULL)
    EXECUTE FUNCTION public.update_last_modified();