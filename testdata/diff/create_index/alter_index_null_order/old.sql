CREATE TABLE public.events (
    id INTEGER PRIMARY KEY,
    occurred_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_events_desc ON public.events (occurred_at DESC);
