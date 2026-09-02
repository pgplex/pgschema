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

CREATE MATERIALIZED VIEW public.user_directory AS
SELECT id, full_name FROM public.users;

CREATE INDEX idx_user_dir_normalized ON public.user_directory (normalize_name(full_name));
