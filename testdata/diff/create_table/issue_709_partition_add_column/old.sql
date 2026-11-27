CREATE TABLE public.events (
    id bigint NOT NULL,
    legacy text,
    amount integer,
    created_at date NOT NULL
) PARTITION BY RANGE (created_at);

CREATE TABLE public.events_2026_01 PARTITION OF public.events
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE public.events_2026_02 PARTITION OF public.events
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
