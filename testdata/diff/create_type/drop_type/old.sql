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

CREATE TYPE public.period AS RANGE (
   subtype = date
);

-- Columns using the dropped types go away, or change type, in the same
-- migration: they must be altered before DROP TYPE (issue #710). A multirange
-- column holds its range the same way.
CREATE TABLE public.tickets (
    id integer PRIMARY KEY,
    priority public.priority,
    severity public.severity,
    busy public.period_multirange[]
);
