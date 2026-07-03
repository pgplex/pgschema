CREATE TABLE public.events (
    id bigint NOT NULL,
    region text NOT NULL,
    payload text
) PARTITION BY LIST (region);

CREATE TABLE public.events_us PARTITION OF public.events FOR VALUES IN ('us');
CREATE TABLE public.events_eu PARTITION OF public.events FOR VALUES IN ('eu');
CREATE TABLE public.events_other PARTITION OF public.events DEFAULT;
