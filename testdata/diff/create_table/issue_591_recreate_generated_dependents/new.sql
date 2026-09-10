DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_reader') THEN
        CREATE ROLE app_reader;
    END IF;
END $$;

CREATE FUNCTION public.orders_audit() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RETURN NEW; END $$;

CREATE TABLE public.orders (
    id integer PRIMARY KEY,
    qty integer NOT NULL,
    price integer NOT NULL,
    total integer GENERATED ALWAYS AS (qty * price) STORED,
    code text GENERATED ALWAYS AS ('ORD-' || id::text) STORED,
    CONSTRAINT orders_total_excl EXCLUDE USING btree ((total + 0) WITH =)
);

CREATE UNIQUE INDEX orders_code_key ON public.orders (code);

CREATE INDEX orders_lookup_idx ON public.orders (total);

CREATE TABLE public.shipments (
    id integer PRIMARY KEY,
    order_code text,
    CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES public.orders (code)
);

CREATE TABLE public.returns (
    id integer PRIMARY KEY,
    order_code text,
    CONSTRAINT returns_order_code_fkey FOREIGN KEY (order_code) REFERENCES public.orders (code)
);

CREATE VIEW public.order_totals AS SELECT id, total FROM public.orders;

CREATE VIEW public.big_orders AS SELECT id FROM public.order_totals WHERE total > 100;

CREATE VIEW public.order_prices AS SELECT id, price FROM public.orders;

CREATE VIEW public.order_labels AS SELECT id, 'x'::text AS label FROM public.orders;

ALTER TABLE public.orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY orders_big ON public.orders USING (total > 100);

CREATE TRIGGER orders_total_trg AFTER UPDATE ON public.orders FOR EACH ROW WHEN (NEW.total > 0) EXECUTE FUNCTION public.orders_audit();

GRANT SELECT (id, total) ON public.orders TO app_reader;

GRANT SELECT ON public.order_totals TO app_reader;
