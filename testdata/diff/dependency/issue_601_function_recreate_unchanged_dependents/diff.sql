DROP TRIGGER IF EXISTS orders_audit ON orders;

DROP POLICY IF EXISTS orders_tenant ON orders;

ALTER TABLE orders DROP CONSTRAINT IF EXISTS orders_qty_check;

DROP INDEX IF EXISTS orders_code_key;

DROP INDEX IF EXISTS orders_large_idx;

ALTER TABLE orders ALTER COLUMN tenant_id DROP DEFAULT;

ALTER TABLE orders ALTER COLUMN priority DROP DEFAULT;

DROP MATERIALIZED VIEW IF EXISTS order_codes RESTRICT;

DROP VIEW IF EXISTS my_orders RESTRICT;

DROP FUNCTION IF EXISTS current_tenant();

CREATE OR REPLACE FUNCTION current_tenant()
RETURNS bigint
LANGUAGE sql
STABLE
AS $$ SELECT current_setting('app.tenant', true)::bigint
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

CREATE OR REPLACE VIEW my_orders AS
 SELECT id,
    code
   FROM orders
  WHERE tenant_id = current_tenant();

CREATE MATERIALIZED VIEW IF NOT EXISTS order_codes AS
 SELECT id,
    code
   FROM orders;

CREATE INDEX IF NOT EXISTS order_codes_normalized_idx ON order_codes (normalize_code(code));

DROP FUNCTION IF EXISTS audit_threshold();

CREATE OR REPLACE FUNCTION audit_threshold()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 10
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

DROP FUNCTION IF EXISTS standalone();

CREATE OR REPLACE FUNCTION standalone()
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT 1
$$;

ALTER TABLE orders ALTER COLUMN tenant_id SET DEFAULT current_tenant();

ALTER TABLE orders ALTER COLUMN priority SET DEFAULT default_priority();

ALTER TABLE orders
ADD CONSTRAINT orders_qty_check CHECK (qty <= max_qty()) NOT VALID;

CREATE OR REPLACE TRIGGER orders_audit
    BEFORE INSERT ON orders
    FOR EACH ROW
    WHEN (((NEW.qty > audit_threshold())))
    EXECUTE FUNCTION mark_audited();

CREATE POLICY orders_tenant ON orders TO tenant_reader USING (tenant_id = current_tenant());

CREATE UNIQUE INDEX IF NOT EXISTS orders_code_key ON orders (normalize_code(code));

COMMENT ON INDEX orders_code_key IS 'Case-insensitive order code';

CREATE INDEX IF NOT EXISTS orders_large_idx ON orders (id) WHERE qty > (max_qty() / 2);

ALTER TABLE orders VALIDATE CONSTRAINT orders_qty_check;
