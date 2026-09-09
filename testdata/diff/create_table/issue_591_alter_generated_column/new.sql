CREATE TABLE public.metrics (
    id integer PRIMARY KEY,
    a integer NOT NULL,
    doubled integer GENERATED ALWAYS AS (a * 2) STORED,
    label text DEFAULT 'none',
    tripled integer GENERATED ALWAYS AS (a * 3) STORED,
    CONSTRAINT metrics_tripled_check CHECK (tripled > 0),
    CONSTRAINT metrics_tripled_key UNIQUE (tripled)
);

CREATE INDEX metrics_doubled_idx ON public.metrics (doubled);

CREATE INDEX metrics_tripled_idx ON public.metrics ((tripled + 1)) WHERE tripled > 10;

CREATE TABLE public.metric_refs (
    tripled integer,
    CONSTRAINT metric_refs_tripled_fkey FOREIGN KEY (tripled) REFERENCES public.metrics (tripled)
);
