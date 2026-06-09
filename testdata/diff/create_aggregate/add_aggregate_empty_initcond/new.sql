-- Aggregate with an explicit empty-string INITCOND, which is semantically distinct
-- from a NULL initial condition and must be preserved (not dropped).
CREATE FUNCTION str_append(text, text) RETURNS text
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT $1 || $2 $$;

CREATE AGGREGATE str_concat(text) (
    SFUNC = str_append,
    STYPE = text,
    INITCOND = ''
);
