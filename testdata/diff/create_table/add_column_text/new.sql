CREATE TABLE public.users (
    id integer NOT NULL,
    name text,
    email text,
    sort_key text COLLATE "C" NOT NULL DEFAULT ''
);