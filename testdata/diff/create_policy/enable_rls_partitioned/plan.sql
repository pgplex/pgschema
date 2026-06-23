ALTER TABLE events ENABLE ROW LEVEL SECURITY;

CREATE POLICY events_org_isolation ON events TO PUBLIC USING (org_id = (NULLIF(current_setting('app.current_org_id', true), ''::text))::uuid);
