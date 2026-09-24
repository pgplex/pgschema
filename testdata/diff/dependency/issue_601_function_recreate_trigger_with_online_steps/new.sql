-- Function whose return type changes (DROP + CREATE)
CREATE FUNCTION audit_threshold() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 10 $$;
CREATE FUNCTION mark_audited() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.audited := true; RETURN NEW; END $$;

-- Trigger WHEN calling it; the plan also changes another table with online
-- steps (a CHECK constraint validated on its own, an index built concurrently),
-- which must not separate the trigger's DROP from its re-creation
CREATE TABLE audited (
    id integer PRIMARY KEY,
    qty integer,
    audited boolean DEFAULT false
);
CREATE TRIGGER audited_mark BEFORE INSERT ON audited FOR EACH ROW WHEN (NEW.qty > audit_threshold()) EXECUTE FUNCTION mark_audited();

CREATE TABLE ledger (
    id integer PRIMARY KEY,
    amount integer,
    CONSTRAINT ledger_amount_check CHECK (amount >= 0)
);
CREATE INDEX ledger_amount_idx ON ledger (amount);
