CREATE OR REPLACE FUNCTION public.is_active(boolean, timestamptz)
RETURNS boolean AS $$
BEGIN
    RETURN $1 AND ($2 IS NULL OR $2 > now());
END;
$$ LANGUAGE plpgsql IMMUTABLE;

CREATE TABLE public.accounts (
    id serial PRIMARY KEY,
    active boolean NOT NULL DEFAULT true,
    expires_at timestamptz
);

CREATE INDEX idx_accounts_active ON public.accounts (id) WHERE is_active(active, expires_at);
