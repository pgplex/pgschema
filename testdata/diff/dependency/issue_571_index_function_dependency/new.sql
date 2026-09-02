-- A table's index (its partial WHERE predicate) can depend on a new function
-- just like a column default can. Indexes are emitted immediately alongside
-- their table, so widgets must be deferred until after "MyFunc" exists, even
-- though no column default/generated expression/CHECK constraint references it.

CREATE OR REPLACE FUNCTION public."MyFunc"(val integer)
RETURNS integer AS $$
BEGIN
    RETURN val * 2;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE public.widgets (
    id serial PRIMARY KEY,
    code integer,
    active boolean NOT NULL DEFAULT true
);

CREATE INDEX widgets_active_partial_idx ON public.widgets (id) WHERE "MyFunc"(code) > 0;
