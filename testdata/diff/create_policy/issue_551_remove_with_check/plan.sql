DROP POLICY IF EXISTS agents_app_tenant ON agents;

CREATE POLICY agents_app_tenant ON agents TO PUBLIC USING (organization_id = '00000000-0000-0000-0000-000000000001'::uuid);
