DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_role') THEN
        CREATE ROLE app_role;
    END IF;
END $$;

CREATE TYPE floatrange AS RANGE (subtype = double precision);

GRANT USAGE ON TYPE floatrange TO app_role;
