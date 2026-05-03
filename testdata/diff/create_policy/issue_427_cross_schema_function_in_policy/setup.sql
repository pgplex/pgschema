-- Setup: Simulate Supabase auth schema with auth.uid() and auth.role()
-- This reproduces GitHub Issue #427 where cross-schema function references
-- in RLS policy expressions lose their schema qualification.
--
-- When search_path includes the auth schema, pg_get_expr() omits the
-- "auth." prefix from function calls, resulting in broken expressions
-- like "uid()" instead of "auth.uid()".

-- Create auth schema (simulating Supabase)
DROP SCHEMA IF EXISTS auth CASCADE;
CREATE SCHEMA auth;

-- Create auth.uid() function (simulating Supabase)
CREATE FUNCTION auth.uid() RETURNS uuid
    LANGUAGE sql STABLE
    AS $$ SELECT '00000000-0000-0000-0000-000000000000'::uuid; $$;

-- Create auth.role() function (simulating Supabase)
CREATE FUNCTION auth.role() RETURNS text
    LANGUAGE sql STABLE
    AS $$ SELECT 'authenticated'::text; $$;

-- Create authenticated role (simulating Supabase)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN
        CREATE ROLE authenticated;
    END IF;
END
$$;
