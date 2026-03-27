DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user;
    END IF;
END $$;

CREATE TYPE entity_kind AS ENUM ('person', 'company', 'organization');

CREATE FUNCTION create_entity(p_name text, p_kind entity_kind)
RETURNS uuid
LANGUAGE sql
AS $$ SELECT gen_random_uuid(); $$;
