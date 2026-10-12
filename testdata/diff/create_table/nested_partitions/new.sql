CREATE TABLE public.events (
    bucket integer NOT NULL,
    label text
) PARTITION BY RANGE (bucket);

CREATE TABLE public.events_early PARTITION OF public.events (
    label DEFAULT 'pending'
) FOR VALUES FROM (0) TO (10) PARTITION BY RANGE (bucket);

CREATE TABLE public.events_first PARTITION OF public.events_early FOR VALUES FROM (0) TO (5);
CREATE TABLE public.events_second PARTITION OF public.events_early FOR VALUES FROM (5) TO (10);
