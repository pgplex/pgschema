-- The partition's default calls a SQL wrapper around a function that reads
-- the parent, so the parent, both functions and the partition keep that order
-- even though the parent's key is postponed until the partition exists
-- (issue #653). A function or procedure that nothing else needs waits for the
-- key, and so does any policy whose expression relies on the key.
CREATE TABLE public.events (
    id integer PRIMARY KEY,
    payload text
) PARTITION BY RANGE (id);

CREATE FUNCTION public.count_events() RETURNS text
LANGUAGE sql
BEGIN ATOMIC
    SELECT count(*)::text FROM public.events;
END;

CREATE FUNCTION public.count_events_text() RETURNS text
LANGUAGE sql
BEGIN ATOMIC
    SELECT public.count_events();
END;

CREATE TABLE public.events_leaf PARTITION OF public.events (
    payload DEFAULT public.count_events_text()
) FOR VALUES FROM (0) TO (10);

CREATE FUNCTION public.upsert_event(p_id integer, p_payload text) RETURNS void
LANGUAGE sql
BEGIN ATOMIC
    INSERT INTO public.events (id, payload) VALUES (p_id, p_payload)
    ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = EXCLUDED.payload;
END;

CREATE PROCEDURE public.upsert_event_proc(p_id integer, p_payload text)
LANGUAGE sql
BEGIN ATOMIC
    INSERT INTO public.events (id, payload) VALUES (p_id, p_payload)
    ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET payload = EXCLUDED.payload;
END;

ALTER TABLE public.events ENABLE ROW LEVEL SECURITY;

CREATE POLICY events_grouped ON public.events
    USING (EXISTS (SELECT payload FROM public.events GROUP BY id));

-- A policy on another table may rely on the parent's key as well.
CREATE TABLE public.event_notes (
    event_id integer NOT NULL REFERENCES public.events (id),
    note text
);

ALTER TABLE public.event_notes ENABLE ROW LEVEL SECURITY;

CREATE POLICY event_notes_grouped ON public.event_notes
    USING (EXISTS (SELECT payload FROM public.events GROUP BY id));
