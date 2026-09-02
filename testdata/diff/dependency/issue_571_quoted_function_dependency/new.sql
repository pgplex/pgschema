-- Quoted, case-sensitive function names must be recognized as dependencies
-- in every place a table or function can reference one:
--   * a column default ("MyFunc" before widgets)
--   * an expression index column and a partial index WHERE predicate
--     (indexes are emitted immediately with their table, so widgets must
--     still be deferred even if no column referenced "MyFunc")
--   * a function body ("ZHelper" before "AWrapper", which calls it)
-- Without quote-aware dependency scanning, alphabetical order would create
-- "AWrapper" and widgets first and fail.

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

CREATE OR REPLACE FUNCTION public."MyFunc"(val integer DEFAULT 0)
RETURNS integer AS $$
BEGIN
    RETURN val * 2;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE public.widgets (
    id serial PRIMARY KEY,
    code integer DEFAULT public."MyFunc"()
);

CREATE INDEX widgets_code_myfunc_idx ON public.widgets ("MyFunc"(code));

CREATE INDEX widgets_active_partial_idx ON public.widgets (id) WHERE "MyFunc"(code) > 0;
