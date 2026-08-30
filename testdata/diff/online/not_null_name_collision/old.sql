CREATE TABLE public.users (
    id integer NOT NULL,
    email text,
    CONSTRAINT users_email_not_null CHECK (email IS NOT NULL)
);
