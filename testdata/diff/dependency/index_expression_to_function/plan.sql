CREATE OR REPLACE FUNCTION normalize_name(
    text
)
RETURNS text
LANGUAGE plpgsql
IMMUTABLE
AS $_$
BEGIN
    RETURN lower(trim($1));
END;
$_$;

CREATE TABLE IF NOT EXISTS users (
    id SERIAL,
    full_name text NOT NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_users_normalized ON users (normalize_name(full_name));
