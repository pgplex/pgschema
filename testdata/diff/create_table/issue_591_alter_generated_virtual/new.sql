CREATE TABLE public.vt (
    a integer NOT NULL,
    v1 integer GENERATED ALWAYS AS (a * 2) VIRTUAL,
    v2 integer,
    s1 integer GENERATED ALWAYS AS (a * 2) VIRTUAL
);
