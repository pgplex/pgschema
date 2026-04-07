CREATE TABLE public.parent_base (
    id uuid NOT NULL,
    name text NOT NULL
);

ALTER TABLE public.parent_base
ADD CONSTRAINT parent_base_pkey PRIMARY KEY (id);
