CREATE OR REPLACE FUNCTION "ZHelper"(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT upper(input)
$$;

CREATE OR REPLACE FUNCTION "AWrapper"(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT "ZHelper"(input)
$$;
