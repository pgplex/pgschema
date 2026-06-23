CREATE TABLE events (
    id uuid NOT NULL,
    org_id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    PRIMARY KEY (created_at, id)
) PARTITION BY RANGE (created_at);

-- RLS is now enabled with a policy on a partitioned table (issue #471)
ALTER TABLE events ENABLE ROW LEVEL SECURITY;

CREATE POLICY events_org_isolation ON events
    FOR ALL
    TO PUBLIC
    USING (org_id = NULLIF(current_setting('app.current_org_id', true), '')::uuid);
