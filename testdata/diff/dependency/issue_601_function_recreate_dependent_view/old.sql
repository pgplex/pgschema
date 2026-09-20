DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'readonly_role') THEN
        CREATE ROLE readonly_role;
    END IF;
END $$;

CREATE FUNCTION calculate(x integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT x + 1 $$;

-- Unchanged view calling the function whose return type changes
CREATE VIEW answer AS SELECT calculate(4) AS answer;
COMMENT ON VIEW answer IS 'The answer';
GRANT SELECT ON answer TO readonly_role;

-- View stacked on the calling view that calls the function itself too
CREATE VIEW answer_doubled AS SELECT answer * 2 AS doubled, calculate(1) AS base FROM answer;

-- Materialized view calling the function
CREATE MATERIALIZED VIEW answer_cached AS SELECT calculate(5) AS cached;
CREATE INDEX idx_answer_cached ON answer_cached (cached);

-- Function with a return type change but no dependent view
CREATE FUNCTION standalone(x integer)
RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT x $$;
