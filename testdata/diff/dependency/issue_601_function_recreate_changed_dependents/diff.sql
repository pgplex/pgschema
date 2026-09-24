CREATE TABLE IF NOT EXISTS refunds (
    id integer,
    tenant_id integer,
    qty integer,
    CONSTRAINT refunds_pkey PRIMARY KEY (id)
);

ALTER TABLE refunds ENABLE ROW LEVEL SECURITY;

ALTER TABLE shipments DROP CONSTRAINT IF EXISTS shipments_qty_check;

DROP INDEX IF EXISTS shipments_qty_idx;

DROP FUNCTION IF EXISTS current_tenant();

CREATE OR REPLACE FUNCTION current_tenant()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$ SELECT current_setting('app.tenant', true)::bigint
$$;

DROP FUNCTION IF EXISTS default_priority();

CREATE OR REPLACE FUNCTION default_priority()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 5
$$;

DROP FUNCTION IF EXISTS max_qty();

CREATE OR REPLACE FUNCTION max_qty()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 100
$$;

DROP FUNCTION IF EXISTS normalize_code(text);

CREATE OR REPLACE FUNCTION normalize_code(
    input text
)
RETURNS text
LANGUAGE sql
IMMUTABLE
AS $$ SELECT lower(input)
$$;

ALTER TABLE refunds ALTER COLUMN tenant_id SET DEFAULT current_tenant();

ALTER TABLE refunds
ADD CONSTRAINT refunds_qty_check CHECK (qty <= max_qty());

CREATE POLICY refunds_tenant ON refunds TO tenant_reader USING (tenant_id = current_tenant());

CREATE INDEX IF NOT EXISTS refunds_large_idx ON refunds (id) WHERE qty > (max_qty() / 2);

ALTER TABLE shipments ALTER COLUMN priority SET DEFAULT default_priority();

ALTER TABLE shipments
ADD CONSTRAINT shipments_qty_check CHECK (qty > 0 AND qty <= max_qty()) NOT VALID;

CREATE INDEX IF NOT EXISTS shipments_qty_idx ON shipments ((qty * max_qty()));

ALTER TABLE shipments VALIDATE CONSTRAINT shipments_qty_check;

CREATE INDEX IF NOT EXISTS shipments_code_idx ON shipments (normalize_code(id::text));
