CREATE TABLE public.users (
    id integer NOT NULL,
    username text NOT NULL,
    email text NOT NULL,
    phone text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);
