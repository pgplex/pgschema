CREATE TABLE public.accounts (
    id integer NOT NULL,
    tenant_id integer NOT NULL,
    email text NOT NULL,
    display_name text,
    CONSTRAINT accounts_pkey PRIMARY KEY (id) INCLUDE (email, display_name),
    CONSTRAINT accounts_tenant_email_key UNIQUE (tenant_id, email)
);

CREATE TABLE public.events (
    id integer NOT NULL,
    occurred_at timestamp NOT NULL,
    payload text,
    CONSTRAINT events_pkey PRIMARY KEY (id, occurred_at) INCLUDE (payload)
) PARTITION BY RANGE (occurred_at);

CREATE TABLE public.events_2026 PARTITION OF public.events
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

CREATE TABLE public.sessions (
    id integer NOT NULL,
    account_id integer NOT NULL,
    token text NOT NULL,
    expires_at timestamp NOT NULL,
    CONSTRAINT sessions_pkey PRIMARY KEY (id),
    CONSTRAINT sessions_token_key UNIQUE (token) INCLUDE (account_id, expires_at),
    CONSTRAINT sessions_account_id_fkey FOREIGN KEY (account_id) REFERENCES public.accounts (id)
);
