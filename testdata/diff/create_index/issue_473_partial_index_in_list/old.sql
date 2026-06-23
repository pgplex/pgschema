CREATE TABLE findings (
    id uuid PRIMARY KEY,
    org_id uuid NOT NULL,
    status varchar(20) NOT NULL,
    impact_score int
);

-- Partial index written with a natural IN-list predicate. PostgreSQL stores
-- this in the catalog with an array-level cast: "= ANY ((ARRAY[...])::text[])".
CREATE INDEX idx_finding_actionable
    ON findings (org_id, status, impact_score DESC)
    WHERE status IN ('new', 'acknowledged');
