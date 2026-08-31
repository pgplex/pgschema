-- "ZHelper" must be created first because "AWrapper" calls it (via a
-- quoted, case-sensitive identifier). Without quote-aware body dependency
-- scanning, alphabetical order would create "AWrapper" first and fail.

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
