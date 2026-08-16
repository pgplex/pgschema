CREATE TABLE base ();

CREATE DOMAIN d AS base;

CREATE TABLE uses (
    dcol d
);

-- Function signature references table "uses", which itself must be created
-- after domain d (whose base type is table base's row type). The function
-- must therefore come after the last table batch.
CREATE FUNCTION public.uses_check(r uses)
RETURNS boolean LANGUAGE plpgsql AS $$
BEGIN
    RETURN r.dcol IS NOT NULL;
END;
$$;

-- Depends on table "uses" only through the TABLE(...) output column type;
-- these columns are excluded from pg_get_function_arguments and only appear
-- in pg_get_function_result as "TABLE(r uses)".
CREATE FUNCTION public.uses_rows()
RETURNS TABLE(r uses) LANGUAGE plpgsql AS $$
BEGIN
    RETURN;
END;
$$;
