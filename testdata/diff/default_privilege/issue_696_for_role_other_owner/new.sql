-- https://github.com/pgplex/pgschema/issues/696
--
-- Same default privileges FOR ROLE app_owner, plus two new tables:
--   - accounts carries an explicit GRANT that must be emitted: the plan cannot
--     know whether apply will run as app_owner, and for any other role the
--     default never auto-grants it.
--   - audit_log has no grant. The conservative REVOKE for it is still emitted:
--     a no-op when apply runs as another role, and the only thing that
--     withholds app_reader's access when apply runs as app_owner.

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

CREATE TABLE accounts (
    id integer PRIMARY KEY,
    name text NOT NULL
);

GRANT SELECT ON TABLE accounts TO app_reader;

CREATE TABLE audit_log (
    id integer PRIMARY KEY,
    message text
);
