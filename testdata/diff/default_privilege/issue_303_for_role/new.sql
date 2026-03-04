-- Create roles for testing
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'demouser') THEN
        CREATE ROLE demouser;
    END IF;
END $$;

-- Grant default privileges with explicit FOR ROLE (issue #303)
ALTER DEFAULT PRIVILEGES FOR ROLE testuser IN SCHEMA public GRANT SELECT ON TABLES TO demouser;
