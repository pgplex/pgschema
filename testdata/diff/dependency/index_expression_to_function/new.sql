CREATE OR REPLACE FUNCTION public.normalize_name(text)
RETURNS text AS $$
BEGIN
    RETURN lower(trim($1));
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE public.users (
    id serial PRIMARY KEY,
    full_name text NOT NULL
);

CREATE INDEX idx_users_normalized ON public.users (normalize_name(full_name));
