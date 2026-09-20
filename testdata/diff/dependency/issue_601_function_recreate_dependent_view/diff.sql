DROP VIEW IF EXISTS answer_doubled RESTRICT;

DROP MATERIALIZED VIEW IF EXISTS answer_cached RESTRICT;

DROP VIEW IF EXISTS answer RESTRICT;

DROP FUNCTION IF EXISTS calculate(integer);

CREATE OR REPLACE FUNCTION calculate(
    x integer
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT x + 2
$$;

CREATE OR REPLACE VIEW answer AS
 SELECT calculate(4) AS answer;

COMMENT ON VIEW answer IS 'The answer';

CREATE MATERIALIZED VIEW IF NOT EXISTS answer_cached AS
 SELECT calculate(5) AS cached;

CREATE INDEX IF NOT EXISTS idx_answer_cached ON answer_cached (cached);

CREATE OR REPLACE VIEW answer_doubled AS
 SELECT answer * 2 AS doubled,
    calculate(1) AS base
   FROM answer;

CREATE OR REPLACE VIEW answer_next AS
 SELECT calculate(6) AS next;

DROP FUNCTION IF EXISTS standalone(integer);

CREATE OR REPLACE FUNCTION standalone(
    x integer
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT x
$$;

GRANT SELECT ON TABLE answer TO readonly_role;
