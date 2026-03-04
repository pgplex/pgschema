-- Create roles for testing
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'demouser') THEN
        CREATE ROLE demouser;
    END IF;
END $$;

-- No default privileges configured
