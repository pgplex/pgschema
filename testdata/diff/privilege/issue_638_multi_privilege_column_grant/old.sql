DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user;
    END IF;
END $$;

CREATE TABLE t (id int PRIMARY KEY, a int, b int);

CREATE TABLE u (id int PRIMARY KEY, x int);

GRANT INSERT (a, b), UPDATE (a, b) ON t TO app_user;

GRANT SELECT (x) ON u TO app_user;
