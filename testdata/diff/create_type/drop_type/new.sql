CREATE TYPE public.status AS ENUM (
   'active',
   'inactive',
   'pending'
);

CREATE TABLE public.tickets (
    id integer PRIMARY KEY,
    severity text
);
