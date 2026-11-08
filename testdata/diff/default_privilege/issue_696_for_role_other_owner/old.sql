-- https://github.com/pgplex/pgschema/issues/696
--
-- Default privileges declared FOR ROLE app_owner, where app_owner is NOT the
-- role that runs pgschema. Objects created by pgschema are owned by the
-- connecting role, so these defaults never fire for them.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_owner') THEN
        CREATE ROLE app_owner;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_reader') THEN
        CREATE ROLE app_reader;
    END IF;
END $$;

-- Grant membership so the current user may declare defaults for app_owner
DO $$
BEGIN
    EXECUTE format('GRANT app_owner TO %I', current_user);
END $$;

ALTER DEFAULT PRIVILEGES FOR ROLE app_owner IN SCHEMA public GRANT SELECT ON TABLES TO app_reader;
