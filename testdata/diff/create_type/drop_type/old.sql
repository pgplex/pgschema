CREATE TYPE public.status AS ENUM (
   'active',
   'inactive',
   'pending'
);

CREATE TYPE public.priority AS ENUM (
   'low',
   'medium',
   'high',
   'urgent'
);

CREATE TYPE public.severity AS ENUM (
   'minor',
   'major'
);

-- Columns using the dropped types go away, or change type, in the same
-- migration: they must be altered before DROP TYPE (issue #710)
CREATE TABLE public.tickets (
    id integer PRIMARY KEY,
    priority public.priority,
    severity public.severity
);
