CREATE FUNCTION numeric_accum(numeric[], numeric) RETURNS numeric[]
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT array_append($1, $2) $$;

CREATE FUNCTION numeric_first(numeric[]) RETURNS numeric
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT $1[1] $$;

CREATE AGGREGATE my_agg(numeric) (
    SFUNC = numeric_accum,
    STYPE = numeric[],
    FINALFUNC = numeric_first,
    INITCOND = '{}'
);

-- SQL-language function calling the new aggregate. PostgreSQL resolves the
-- call at creation, so the function must be created after the aggregate.
CREATE FUNCTION first_of(vals numeric[]) RETURNS numeric
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT my_agg(x) FROM unnest(vals) AS u(x) $$;

-- Aggregate whose SQL transition function calls the first aggregate: the
-- required order is my_agg -> b_sfunc -> b_agg.
CREATE FUNCTION b_sfunc(state numeric, x numeric) RETURNS numeric
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT state + my_agg(v) FROM unnest(ARRAY[x]) AS u(v) $$;

CREATE AGGREGATE b_agg(numeric) (
    SFUNC = b_sfunc,
    STYPE = numeric,
    INITCOND = '0'
);
