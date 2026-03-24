CREATE FUNCTION has_scope(p_scope text) RETURNS boolean
LANGUAGE sql STABLE AS $$ SELECT p_scope IS NOT NULL $$;

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    tenant_id INTEGER NOT NULL
);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;

CREATE POLICY user_tenant_isolation ON users
    FOR ALL
    TO PUBLIC
    USING (tenant_id = 1);

-- Policy with string literal containing schema prefix (Issue #371)
-- This must NOT produce a false-positive diff
CREATE POLICY scope_check ON users
    FOR SELECT
    TO PUBLIC
    USING (has_scope('public.manage'));