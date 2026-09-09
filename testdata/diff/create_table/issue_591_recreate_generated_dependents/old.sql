DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_reader') THEN
        CREATE ROLE app_reader;
    END IF;
END $$;

CREATE TABLE public.orders (
    id integer PRIMARY KEY,
    qty integer NOT NULL,
    price integer NOT NULL,
    total integer,
    code text
);

CREATE UNIQUE INDEX orders_code_key ON public.orders (code);

CREATE INDEX orders_lookup_idx ON public.orders (price);

CREATE TABLE public.shipments (
    id integer PRIMARY KEY,
    order_code text,
    CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES public.orders (code)
);

CREATE VIEW public.order_totals AS SELECT id, total FROM public.orders;

CREATE VIEW public.big_orders AS SELECT id FROM public.order_totals WHERE total > 100;

CREATE VIEW public.order_prices AS SELECT id, price FROM public.orders;

GRANT SELECT (id, total) ON public.orders TO app_reader;
