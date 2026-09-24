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
CREATE FUNCTION default_priority() RETURNS bigint LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

-- Dependents that start calling the functions, or are added, in the desired state
CREATE TABLE shipments (
    id integer PRIMARY KEY,
    qty integer,
    priority integer DEFAULT default_priority(),
    CONSTRAINT shipments_qty_check CHECK (qty > 0 AND qty <= max_qty())
);
CREATE INDEX shipments_qty_idx ON shipments ((qty * max_qty()));
CREATE INDEX shipments_code_idx ON shipments ((normalize_code(id::text)));

-- New table whose default, CHECK constraint, index and policy call recreated functions
CREATE TABLE refunds (
    id integer PRIMARY KEY,
    tenant_id integer DEFAULT current_tenant(),
    qty integer,
    CONSTRAINT refunds_qty_check CHECK (qty <= max_qty())
);
CREATE INDEX refunds_large_idx ON refunds (id) WHERE qty > max_qty() / 2;
ALTER TABLE refunds ENABLE ROW LEVEL SECURITY;
CREATE POLICY refunds_tenant ON refunds TO tenant_reader USING (tenant_id = current_tenant());
