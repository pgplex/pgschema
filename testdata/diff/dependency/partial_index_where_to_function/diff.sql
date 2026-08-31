CREATE OR REPLACE FUNCTION is_active(
    boolean,
    timestamp with time zone
)
RETURNS boolean
LANGUAGE plpgsql
IMMUTABLE
AS $_$
BEGIN
    RETURN $1 AND ($2 IS NULL OR $2 > now());
END;
$_$;

CREATE TABLE IF NOT EXISTS accounts (
    id SERIAL,
    active boolean DEFAULT true NOT NULL,
    expires_at timestamptz,
    CONSTRAINT accounts_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_accounts_active ON accounts (id) WHERE (public.is_active(active, expires_at));
