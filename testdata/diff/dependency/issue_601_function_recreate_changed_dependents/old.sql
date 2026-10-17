DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'tenant_reader') THEN
        CREATE ROLE tenant_reader;
    END IF;
END $$;

-- Functions whose return type or parameter names change (DROP + CREATE)
CREATE FUNCTION max_qty() RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT 100 $$;
CREATE FUNCTION normalize_code(code text) RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT lower(code) $$;
CREATE FUNCTION current_tenant() RETURNS integer LANGUAGE sql STABLE AS $$ SELECT current_setting('app.tenant', true)::integer $$;
CREATE FUNCTION default_priority() RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT 5 $$;

-- Dependents that start calling the functions, or are added, in the desired state
CREATE TABLE shipments (
    id integer PRIMARY KEY,
    qty integer,
    priority integer DEFAULT 0,
    CONSTRAINT shipments_qty_check CHECK (qty > 0)
);
CREATE INDEX shipments_qty_idx ON shipments (qty);
