CREATE FUNCTION public.random_id()
RETURNS bigint LANGUAGE sql AS $$
    SELECT CAST(1000000000 + floor(random() * 9000000000) AS bigint);
$$;

CREATE TABLE public.x (
    id bigint PRIMARY KEY DEFAULT random_id()
);

-- Depends on table x only through the TABLE(...) output column type; these
-- columns are excluded from pg_get_function_arguments and only appear in
-- pg_get_function_result as "TABLE(r x)".
CREATE FUNCTION public.x_rows()
RETURNS TABLE(r x) LANGUAGE plpgsql AS $$
BEGIN
    RETURN;
END;
$$;
