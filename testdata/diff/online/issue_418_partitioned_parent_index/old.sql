CREATE TABLE public.events (
    id bigint NOT NULL,
    occurred timestamp with time zone NOT NULL,
    payload jsonb
) PARTITION BY RANGE (occurred);

CREATE TABLE public.events_2026_04 PARTITION OF public.events
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
