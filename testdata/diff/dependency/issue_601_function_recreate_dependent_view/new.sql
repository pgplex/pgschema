DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'readonly_role') THEN
        CREATE ROLE readonly_role;
    END IF;
END $$;

CREATE FUNCTION calculate(x integer)
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT x + 2 $$;

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
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT x $$;

-- New view calling the recreated function: must not bind to the old function
CREATE VIEW answer_next AS SELECT calculate(6) AS next;

-- Function with a return type change that only a modified view starts calling:
-- modified first, the view would bind to the old function and block its drop
CREATE FUNCTION label_value(x integer)
RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT x $$;
CREATE VIEW answer_label AS SELECT 'answer'::text AS label, label_value(7) AS value;
