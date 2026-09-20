CREATE TYPE public.status AS ENUM (
   'active',
   'inactive',
   'pending'
);

CREATE TABLE public.work (
    id integer PRIMARY KEY,
    state public.status NOT NULL DEFAULT 'pending'
);
