DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tenant_reader') THEN
        CREATE ROLE tenant_reader;
    END IF;
END $$;

-- Functions whose return type or parameter names change (DROP + CREATE)
CREATE FUNCTION max_qty() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 100 $$;
CREATE FUNCTION normalize_code(input text) RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT lower(input) $$;
CREATE FUNCTION current_tenant() RETURNS bigint LANGUAGE sql STABLE AS $$ SELECT current_setting('app.tenant', true)::bigint $$;
CREATE FUNCTION audit_threshold() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 10 $$;
CREATE FUNCTION default_priority() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

CREATE FUNCTION mark_audited() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN NEW.audited := true; RETURN NEW; END $$;

-- Unchanged table: column defaults, CHECK constraint, unique expression index
-- with a comment, partial index, policy and trigger WHEN calling them
CREATE TABLE orders (
    id integer PRIMARY KEY,
    tenant_id integer NOT NULL DEFAULT current_tenant(),
    code text NOT NULL,
    qty integer NOT NULL,
    priority integer DEFAULT default_priority(),
    audited boolean DEFAULT false,
    CONSTRAINT orders_qty_check CHECK (qty <= max_qty())
);
CREATE UNIQUE INDEX orders_code_key ON orders ((normalize_code(code)));
COMMENT ON INDEX orders_code_key IS 'Case-insensitive order code';
CREATE INDEX orders_large_idx ON orders (id) WHERE qty > max_qty() / 2;
ALTER TABLE orders ENABLE ROW LEVEL SECURITY;
CREATE POLICY orders_tenant ON orders TO tenant_reader USING (tenant_id = current_tenant());
CREATE TRIGGER orders_audit BEFORE INSERT ON orders FOR EACH ROW WHEN (NEW.qty > audit_threshold()) EXECUTE FUNCTION mark_audited();

-- View calling a recreated function that table defaults call as well
CREATE VIEW my_orders AS SELECT id, code FROM orders WHERE tenant_id = current_tenant();

-- Materialized view whose index, not its query, calls a recreated function
CREATE MATERIALIZED VIEW order_codes AS SELECT id, code FROM orders;
CREATE INDEX order_codes_normalized_idx ON order_codes ((normalize_code(code)));

-- Function with a return type change but no dependents
CREATE FUNCTION standalone() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 1 $$;
