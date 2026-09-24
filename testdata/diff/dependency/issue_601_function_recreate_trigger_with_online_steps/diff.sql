ALTER TABLE ledger
ADD CONSTRAINT ledger_amount_check CHECK (amount >= 0);

CREATE INDEX IF NOT EXISTS ledger_amount_idx ON ledger (amount);

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
