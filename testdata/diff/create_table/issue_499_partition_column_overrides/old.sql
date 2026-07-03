CREATE TABLE public.orders (
    id bigint NOT NULL,
    region text NOT NULL,
    priority integer DEFAULT 0,
    notes text
) PARTITION BY LIST (region);

CREATE TABLE public.orders_eu PARTITION OF public.orders
    FOR VALUES IN ('eu');
