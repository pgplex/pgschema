CREATE TABLE public.parent_base (
    id uuid NOT NULL,
    name text NOT NULL,
    CONSTRAINT parent_base_pkey PRIMARY KEY (id),
    CONSTRAINT no_direct_insert CHECK (false) NO INHERIT
);
