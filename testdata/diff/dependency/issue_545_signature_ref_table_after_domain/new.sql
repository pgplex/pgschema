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
