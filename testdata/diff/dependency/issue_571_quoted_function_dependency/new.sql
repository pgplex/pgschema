-- Quoted, case-sensitive function names must be recognized as dependencies
-- both in column defaults ("MyFunc" before widgets) and in function bodies
-- ("ZHelper" before "AWrapper", which calls it). Without quote-aware
-- dependency scanning, alphabetical order would create "AWrapper" first
-- and fail.

CREATE FUNCTION "ZHelper"(input text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT upper(input) $$;

CREATE FUNCTION "AWrapper"(input text)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT "ZHelper"(input) $$;

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
