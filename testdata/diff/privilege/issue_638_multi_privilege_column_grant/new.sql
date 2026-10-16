DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_user') THEN
        CREATE ROLE app_user;
    END IF;
END $$;

CREATE TABLE t (id int PRIMARY KEY, a int, b int);

CREATE TABLE u (id int PRIMARY KEY, x int);

CREATE TABLE v (id int PRIMARY KEY, c int);

CREATE TABLE w (id int PRIMARY KEY, c int);

GRANT INSERT (a), UPDATE (a) ON t TO app_user;

GRANT INSERT (x), SELECT (x), UPDATE (x) ON u TO app_user;

GRANT INSERT (c), UPDATE (c) ON v TO app_user WITH GRANT OPTION;

GRANT INSERT (c), UPDATE (c) ON w TO app_user;
