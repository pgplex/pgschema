-- The partition's default calls a SQL function that reads the parent, so the
-- parent, the function and the partition keep that order even though the
-- parent's key is postponed until the partition exists (issue #653). A
-- function that nothing else needs waits for the key, and so does the
-- parent's policy, whose expression relies on the key.
CREATE TABLE public.events (
    id integer PRIMARY KEY,
    payload text
) PARTITION BY RANGE (id);

CREATE FUNCTION public.count_events() RETURNS text
LANGUAGE sql
BEGIN ATOMIC
    SELECT count(*)::text FROM public.events;
END;

CREATE TABLE public.events_leaf PARTITION OF public.events (
    payload DEFAULT public.count_events()
) FOR VALUES FROM (0) TO (10);

CREATE FUNCTION public.upsert_event(p_id integer, p_payload text) RETURNS void
LANGUAGE sql
BEGIN ATOMIC
    INSERT INTO public.events (id, payload) VALUES (p_id, p_payload)
    ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = EXCLUDED.payload;
END;

ALTER TABLE public.events ENABLE ROW LEVEL SECURITY;

CREATE POLICY events_grouped ON public.events
    USING (EXISTS (SELECT payload FROM public.events GROUP BY id));
