CREATE OR REPLACE FUNCTION str_append(
    text,
    text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $_$ SELECT $1 || $2
$_$;

CREATE AGGREGATE str_concat(text) (
    SFUNC = str_append,
    STYPE = text,
    INITCOND = ''
);
