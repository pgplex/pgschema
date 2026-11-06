CREATE TABLE IF NOT EXISTS invoices (
    id integer,
    CONSTRAINT invoices_pkey PRIMARY KEY (id),
    CONSTRAINT invoices_id_check CHECK (id > 0)
);

COMMENT ON CONSTRAINT invoices_pkey ON invoices IS 'invoice identifier';

COMMENT ON CONSTRAINT invoices_id_check ON invoices IS 'positive identifiers only';

COMMENT ON CONSTRAINT customers_email_key ON customers IS NULL;

ALTER TABLE orders
ADD CONSTRAINT orders_customer_total_key UNIQUE (customer_id, total);

COMMENT ON CONSTRAINT orders_customer_total_key ON orders IS 'added with its constraint';

COMMENT ON CONSTRAINT orders_customer_id_fkey ON orders IS 'owning customer';

COMMENT ON CONSTRAINT orders_total_check ON orders IS 'totals are never negative';

COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';
