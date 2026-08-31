CREATE OR REPLACE FUNCTION public."MyFunc"()
RETURNS integer AS $$
BEGIN
    RETURN 42;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE public.widgets (
    id serial PRIMARY KEY,
    code integer DEFAULT public."MyFunc"()
);
