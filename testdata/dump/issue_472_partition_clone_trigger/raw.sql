--
-- Test case for GitHub issue #472: partition-clone child triggers dumped
--
-- When a FOR EACH ROW trigger is defined on a partitioned table, PostgreSQL
-- automatically clones it onto every partition child, creating one extra
-- pg_trigger row per partition with tgparentid != 0 (and tgisinternal = false).
-- These are internal artifacts: pg_dump never emits them, only the top-level
-- trigger on the partitioned parent (tgparentid = 0).
--
-- pgschema dump used to emit all of them, producing a bogus
-- CREATE OR REPLACE TRIGGER trg_rollup ... ON ledger_2026_06 that also left a
-- dangling reference when the partition was excluded via .pgschemaignore.
--

CREATE TABLE ledger (
    id uuid NOT NULL,
    amount bigint NOT NULL,
    ts timestamptz NOT NULL,
    PRIMARY KEY (ts, id)
) PARTITION BY RANGE (ts);

CREATE TABLE ledger_2026_06 PARTITION OF ledger
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');

CREATE FUNCTION tg_noop() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$;

CREATE TRIGGER trg_rollup AFTER INSERT ON ledger
    FOR EACH ROW EXECUTE FUNCTION tg_noop();
