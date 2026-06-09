-- Aggregate exercising the advanced options: COMBINEFUNC (parallel aggregation),
-- the moving-aggregate group (MSFUNC/MINVFUNC/MSTYPE/MINITCOND), and PARALLEL = SAFE.
CREATE FUNCTION num_add(numeric, numeric) RETURNS numeric
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT $1 + $2 $$;

CREATE FUNCTION num_sub(numeric, numeric) RETURNS numeric
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$ SELECT $1 - $2 $$;

CREATE AGGREGATE my_sum(numeric) (
    SFUNC = num_add,
    STYPE = numeric,
    COMBINEFUNC = num_add,
    INITCOND = '0',
    MSFUNC = num_add,
    MINVFUNC = num_sub,
    MSTYPE = numeric,
    MINITCOND = '0',
    PARALLEL = SAFE
);
