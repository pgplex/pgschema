-- Ordinary table: PK gains INCLUDE, UNIQUE loses it, a second UNIQUE reorders it.
CREATE TABLE public.accounts (
    id integer NOT NULL,
    tenant_id integer NOT NULL,
    email text NOT NULL,
    display_name text,
    "select" text,
    CONSTRAINT accounts_pkey PRIMARY KEY (id),
    CONSTRAINT accounts_tenant_email_key UNIQUE (tenant_id, email) INCLUDE (display_name),
    CONSTRAINT accounts_email_key UNIQUE (email) INCLUDE (display_name, "select")
);

-- Partitioned table: PK gains INCLUDE; the partition's copy follows.
CREATE TABLE public.events (
    id integer NOT NULL,
    occurred_at timestamp NOT NULL,
    payload text,
    CONSTRAINT events_pkey PRIMARY KEY (id, occurred_at)
) PARTITION BY RANGE (occurred_at);

CREATE TABLE public.events_2026 PARTITION OF public.events
    FOR VALUES FROM ('2026-01-01') TO ('2027-01-01');

-- Dropping an included column drops the constraint with it.
CREATE TABLE public.audit (
    id integer NOT NULL,
    note text,
    CONSTRAINT audit_id_key UNIQUE (id) INCLUDE (note)
);

-- Existing table gains a key with INCLUDE and a self-referencing FK.
CREATE TABLE public.node (
    payload text
);
