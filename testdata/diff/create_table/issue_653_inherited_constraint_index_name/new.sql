-- A standalone index on the parent holds the name PostgreSQL would generate
-- for the partition's primary key index, so the partition's copy becomes
-- events_leaf_pkey1 (issue #653). Creating the partition with an inline
-- PRIMARY KEY would hand that generated name to the copy and block the
-- standalone index. Constraints are added after the partitions instead,
-- each copy under its catalog name.
CREATE TABLE public.events (
    id integer PRIMARY KEY,
    region text NOT NULL,
    CONSTRAINT events_region_id_key UNIQUE (region, id)
) PARTITION BY RANGE (id);

CREATE INDEX events_leaf_pkey ON public.events (id);

CREATE TABLE public.events_leaf PARTITION OF public.events FOR VALUES FROM (0) TO (10);

CREATE TABLE public.events_archive PARTITION OF public.events
    FOR VALUES FROM (10) TO (20) PARTITION BY RANGE (id);

CREATE TABLE public.events_archive_old PARTITION OF public.events_archive FOR VALUES FROM (10) TO (15);

-- The foreign key waits for the partitioned primary key to be complete.
CREATE TABLE public.event_notes (
    event_id integer NOT NULL REFERENCES public.events (id),
    note text
);

-- So does a foreign key that targets a partition's copy of the key.
CREATE TABLE public.leaf_notes (
    event_id integer NOT NULL REFERENCES public.events_leaf (id)
);

-- A SQL-standard function body is validated at creation and needs the key.
CREATE FUNCTION public.upsert_event(p_id integer, p_region text) RETURNS void
LANGUAGE sql
BEGIN ATOMIC
    INSERT INTO public.events (id, region) VALUES (p_id, p_region)
    ON CONFLICT ON CONSTRAINT events_pkey DO UPDATE SET region = EXCLUDED.region;
END;
