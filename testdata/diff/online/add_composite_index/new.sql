CREATE TABLE public.users (
    id integer NOT NULL,
    email text,
    username text,
    created_at timestamp with time zone,
    status text
);

CREATE INDEX idx_users_email_status ON public.users (email, status DESC);

CREATE INDEX idx_users_email_include ON public.users (email) INCLUDE (username);