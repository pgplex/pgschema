-- Table with mixed-case SERIAL columns. PostgreSQL auto-creates sequences
-- named orders_orderId_seq and users_userId_seq (preserving case). Their
-- column_default contains quoted identifiers:
--   nextval('public."orders_orderId_seq"'::regclass)
-- The sequence name in pg_sequences is stored unquoted: orders_orderId_seq.
-- pgschema must strip the outer double-quotes before the JOIN so it can
-- detect these sequences as SERIAL-owned and skip emitting DROP SEQUENCE.
CREATE TABLE orders (
    "orderId"  SERIAL PRIMARY KEY,
    amount     numeric(10,2) NOT NULL
);

CREATE TABLE users (
    "userId"   SERIAL PRIMARY KEY,
    email      text NOT NULL
);
