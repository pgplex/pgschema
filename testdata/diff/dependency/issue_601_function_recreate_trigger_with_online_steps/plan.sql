ALTER TABLE ledger
ADD CONSTRAINT ledger_amount_check CHECK (amount >= 0) NOT VALID;

ALTER TABLE ledger VALIDATE CONSTRAINT ledger_amount_check;

CREATE INDEX CONCURRENTLY IF NOT EXISTS ledger_amount_idx ON ledger (amount);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'ledger_amount_idx';

DROP TRIGGER IF EXISTS audited_mark ON audited;

DROP FUNCTION IF EXISTS audit_threshold();

CREATE OR REPLACE FUNCTION audit_threshold()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 10
$$;

CREATE OR REPLACE TRIGGER audited_mark
    BEFORE INSERT ON audited
    FOR EACH ROW
    WHEN (((NEW.qty > audit_threshold())))
    EXECUTE FUNCTION mark_audited();
