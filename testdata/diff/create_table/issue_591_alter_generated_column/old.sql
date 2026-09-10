-- Re-created column with dependents: views, policy, trigger, grants,
-- expression EXCLUDE, standalone unique index bound by FKs, moved index
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
    total integer,
    code text,
    CONSTRAINT orders_total_excl EXCLUDE USING btree ((total + 0) WITH =)
);

CREATE UNIQUE INDEX orders_code_key ON public.orders (code);

CREATE INDEX orders_lookup_idx ON public.orders (price);

CREATE TABLE public.shipments (
    id integer PRIMARY KEY,
    order_code text,
    CONSTRAINT shipments_order_code_fkey FOREIGN KEY (order_code) REFERENCES public.orders (code)
);

CREATE TABLE public.returns (
    id integer PRIMARY KEY,
    order_total integer,
    order_code text
);

CREATE VIEW public.order_totals AS SELECT id, total FROM public.orders;

CREATE VIEW public.big_orders AS SELECT id FROM public.order_totals WHERE total > 100;

CREATE VIEW public.order_prices AS SELECT id, price FROM public.orders;

CREATE VIEW public.order_labels AS SELECT id, code AS label FROM public.orders;

ALTER TABLE public.orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY orders_big ON public.orders USING (total > 100);

CREATE TRIGGER orders_total_trg AFTER UPDATE ON public.orders FOR EACH ROW WHEN (NEW.total > 0) EXECUTE FUNCTION public.orders_audit();

GRANT SELECT (id, total) ON public.orders TO app_reader;

GRANT SELECT ON public.order_totals TO app_reader;

GRANT SELECT (id) ON public.big_orders TO app_reader;

-- STORED expression change, STORED -> plain, plain -> STORED with a
-- check constraint, unique constraint bound by an FK, and an expression index
CREATE TABLE public.metrics (
    id integer PRIMARY KEY,
    a integer NOT NULL,
    doubled integer GENERATED ALWAYS AS (a * 3) STORED,
    label text GENERATED ALWAYS AS ('n=' || a) STORED,
    tripled integer,
    CONSTRAINT metrics_tripled_check CHECK (tripled > 0),
    CONSTRAINT metrics_tripled_key UNIQUE (tripled)
);

CREATE INDEX metrics_doubled_idx ON public.metrics (doubled);

CREATE INDEX metrics_tripled_idx ON public.metrics ((tripled + 1)) WHERE tripled > 10;

CREATE TABLE public.metric_refs (
    tripled integer,
    CONSTRAINT metric_refs_tripled_fkey FOREIGN KEY (tripled) REFERENCES public.metrics (tripled)
);

-- VIRTUAL expression change, VIRTUAL -> plain, STORED -> VIRTUAL (PG18+)
CREATE TABLE public.vt (
    a integer NOT NULL,
    v1 integer GENERATED ALWAYS AS (a * 3) VIRTUAL,
    v2 integer GENERATED ALWAYS AS (a * 2) VIRTUAL,
    s1 integer GENERATED ALWAYS AS (a * 2) STORED
);
