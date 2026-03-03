CREATE TABLE IF NOT EXISTS person_accounts (
    id uuid DEFAULT gen_random_uuid(),
    first_name text,
    last_name text,
    email_address text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    modified_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT person_accounts_pkey PRIMARY KEY (id),
    CONSTRAINT person_accounts_email_address_key UNIQUE (email_address)
);

CREATE OR REPLACE FUNCTION auth_lookup_account_by_email(
    input_email text
)
RETURNS text
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = public, pg_temp
AS $$
    SELECT
        pa.id::text AS person_account_id
    FROM person_accounts pa
    WHERE lower(pa.email_address) = lower(trim(input_email))
    LIMIT 1;
$$;

REVOKE EXECUTE ON FUNCTION auth_lookup_account_by_email(input_email text) FROM PUBLIC;
