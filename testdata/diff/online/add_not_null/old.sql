CREATE TABLE public.users (
    id integer NOT NULL,
    username text NOT NULL,
    email text,
    phone text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

-- Issue #564: a NOT NULL constraint that was added NOT VALID (e.g. apply was
-- interrupted before VALIDATE, or the ADD was run by hand) must be re-validated
-- on the next plan instead of being silently treated as done.
ALTER TABLE public.users ADD CONSTRAINT users_phone_not_null NOT NULL phone NOT VALID;
