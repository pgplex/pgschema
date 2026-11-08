CREATE TABLE public.accounts (
    id integer NOT NULL,
    tenant_id integer NOT NULL,
    email text NOT NULL,
    display_name text,
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_tenant_email_key UNIQUE (tenant_id, email) INCLUDE (display_name)
);

CREATE TABLE public.events (
    id integer NOT NULL,
    occurred_at timestamp NOT NULL,
    payload text,
    CONSTRAINT events_pkey PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE TABLE public.events_2026 PARTITION OF public.events
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');
