ALTER TABLE orders ENABLE ROW LEVEL SECURITY;

CREATE POLICY orders_user_access ON orders FOR SELECT TO PUBLIC USING (user_id IN ( SELECT users.id FROM users));

CREATE POLICY "UserPolicy" ON users TO PUBLIC USING (tenant_id = (current_setting('app.current_tenant'))::integer);

CREATE POLICY admin_only ON users FOR DELETE TO PUBLIC USING (is_admin());

CREATE POLICY "my-policy" ON users FOR INSERT TO PUBLIC WITH CHECK ((role)::text = 'user');

CREATE POLICY "select" ON users FOR SELECT TO PUBLIC USING (true);

CREATE POLICY user_tenant_isolation ON users FOR UPDATE TO PUBLIC USING (tenant_id = (current_setting('app.current_tenant'))::integer);
