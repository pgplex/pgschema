CREATE FUNCTION public.random_id()
RETURNS bigint LANGUAGE sql AS $$
    SELECT CAST(1000000000 + floor(random() * 9000000000) AS bigint);
$$;

CREATE TABLE public.x (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE DEFAULT random_id(),
    name text NOT NULL UNIQUE,
    flag boolean NOT NULL DEFAULT FALSE
);

CREATE FUNCTION public.x_is_flagged(id bigint)
RETURNS boolean LANGUAGE sql STABLE AS $$
    SELECT x.flag FROM x WHERE x.id = id;
$$;
