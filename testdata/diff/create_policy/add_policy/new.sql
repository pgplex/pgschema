CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    tenant_id INTEGER NOT NULL,
    role VARCHAR(50) NOT NULL
);

CREATE TABLE orders (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id),
    total NUMERIC(10,2)
);

-- Function to check if user is admin (for Issue #259)
CREATE FUNCTION is_admin() RETURNS boolean LANGUAGE sql AS $$ SELECT true $$;

-- RLS is enabled with multiple policies demonstrating quoting scenarios
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- RLS on orders with policy referencing users table (Issue #224)
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;

-- Policy with function call in USING clause (Issue #259)
-- Tests that parentheses are correctly preserved around function calls
CREATE POLICY admin_only ON users
    FOR DELETE
    TO PUBLIC
    USING (is_admin());

-- Policy with reserved word name (requires quoting)
CREATE POLICY "select" ON users
    FOR SELECT
    TO PUBLIC
    USING (true);

-- Policy with mixed case name (requires quoting to preserve case)
CREATE POLICY "UserPolicy" ON users
    FOR ALL
    TO PUBLIC
    USING (tenant_id = current_setting('app.current_tenant')::INTEGER);

-- Policy with special character in name (requires quoting)
CREATE POLICY "my-policy" ON users
    FOR INSERT
    TO PUBLIC
    WITH CHECK (role = 'user');

-- Policy with regular snake_case name (no quoting needed)
CREATE POLICY user_tenant_isolation ON users
    FOR UPDATE
    TO PUBLIC
    USING (tenant_id = current_setting('app.current_tenant')::INTEGER);

-- Policy with subquery referencing another table (Issue #224)
-- Tests that same-schema table qualifiers are normalized
CREATE POLICY orders_user_access ON orders
    FOR SELECT
    TO PUBLIC
    USING (user_id IN (SELECT id FROM users));

-- Restrictive policy (Issue #477): AS RESTRICTIVE must be preserved
CREATE POLICY admin_restricted ON users
    AS RESTRICTIVE
    FOR ALL
    TO PUBLIC
    USING (is_admin());
