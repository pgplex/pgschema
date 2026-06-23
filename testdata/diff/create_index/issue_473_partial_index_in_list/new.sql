CREATE TABLE findings (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL,
    status varchar(20) NOT NULL,
    impact_score int
);

-- Same partial index, but written in the form that pgschema dump renders
-- (copied from pg_get_indexdef). When re-parsed, PostgreSQL stores it with an
-- element-level cast: "= ANY (ARRAY[('x'::varchar)::text, ...])". This is
-- semantically identical to the IN-list form in old.sql, so the diff must be
-- empty - no spurious concurrent rebuild (issue #473).
CREATE INDEX idx_finding_actionable
    ON findings (org_id, status, impact_score DESC)
    WHERE (status)::text = ANY ((ARRAY['new'::character varying, 'acknowledged'::character varying])::text[]);
