CREATE OR REPLACE FUNCTION numeric_accum(
    numeric[],
    numeric
)
RETURNS numeric[]
LANGUAGE sql
IMMUTABLE
AS $_$ SELECT array_append($1, $2)
$_$;

CREATE OR REPLACE FUNCTION numeric_first(
    numeric[]
)
RETURNS numeric
LANGUAGE sql
IMMUTABLE
AS $_$ SELECT $1[1]
$_$;

CREATE AGGREGATE my_agg(numeric) (
    SFUNC = numeric_accum,
    STYPE = numeric[],
    FINALFUNC = numeric_first,
    INITCOND = '{}'
);

CREATE OR REPLACE FUNCTION b_sfunc(
    state numeric,
    x numeric
)
RETURNS numeric
LANGUAGE sql
IMMUTABLE
AS $$ SELECT state + my_agg(v) FROM unnest(ARRAY[x]) AS u(v)
$$;

CREATE OR REPLACE FUNCTION first_of(
    vals numeric[]
)
RETURNS numeric
LANGUAGE sql
IMMUTABLE
AS $$ SELECT my_agg(x) FROM unnest(vals) AS u(x)
$$;

CREATE AGGREGATE b_agg(numeric) (
    SFUNC = b_sfunc,
    STYPE = numeric,
    INITCOND = '0'
);
