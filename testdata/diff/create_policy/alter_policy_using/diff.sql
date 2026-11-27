ALTER POLICY scope_gate ON users USING (( SELECT (NOT has_scope('read'))));
ALTER POLICY user_tenant_isolation ON users USING (tenant_id = 2);
