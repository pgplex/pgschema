CREATE TYPE public.status AS ENUM (
   'active',
   'inactive',
   'pending',
   'archived'
);

-- Issue #600: the new label is used in the same plan that adds it, so the
-- ADD VALUE must be committed before this default can reference it
CREATE TABLE public.work (
    id integer PRIMARY KEY,
    state public.status NOT NULL DEFAULT 'archived'
);

-- A newly created table using the new label: ADD VALUE must run (and commit)
-- before the create phase, not after it
CREATE TABLE public.work_archive (
    id integer PRIMARY KEY,
    state public.status NOT NULL DEFAULT 'archived'
);
