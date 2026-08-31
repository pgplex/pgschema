CREATE TABLE IF NOT EXISTS users (
    id SERIAL,
    full_name text NOT NULL,
    CONSTRAINT users_pkey PRIMARY KEY (id)
);

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

CREATE MATERIALIZED VIEW IF NOT EXISTS user_directory AS
 SELECT id,
    full_name
   FROM users;

CREATE INDEX IF NOT EXISTS idx_user_dir_normalized ON user_directory (normalize_name(full_name));
