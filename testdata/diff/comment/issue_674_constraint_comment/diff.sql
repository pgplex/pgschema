ALTER TABLE events DETACH PARTITION events_old;

CREATE TABLE IF NOT EXISTS invoices (
    id integer,
    CONSTRAINT invoices_pkey PRIMARY KEY (id),
    CONSTRAINT invoices_id_check CHECK (id > 0)
);

COMMENT ON CONSTRAINT invoices_pkey ON invoices IS 'invoice identifier';

COMMENT ON CONSTRAINT invoices_id_check ON invoices IS 'positive identifiers only';

ALTER TABLE events DROP CONSTRAINT events_region_fk;

COMMENT ON CONSTRAINT customers_email_key ON customers IS NULL;

ALTER TABLE events DROP CONSTRAINT events_pkey;

ALTER TABLE events
ADD CONSTRAINT events_id_key PRIMARY KEY (id);

COMMENT ON CONSTRAINT events_id_key ON events IS 'parent key';

ALTER TABLE orders
ADD CONSTRAINT orders_customer_total_key UNIQUE (customer_id, total);

COMMENT ON CONSTRAINT orders_customer_total_key ON orders IS 'added with its constraint';

COMMENT ON CONSTRAINT orders_customer_id_fkey ON orders IS 'owning customer';

COMMENT ON CONSTRAINT orders_total_check ON orders IS 'totals are never negative';

ALTER TABLE regions DROP CONSTRAINT regions_pkey;

ALTER TABLE regions
ADD CONSTRAINT regions_id_key PRIMARY KEY (id);

ALTER TABLE events ATTACH PARTITION events_old FOR VALUES FROM (10) TO (30);

ALTER TABLE events
ADD CONSTRAINT events_region_fk FOREIGN KEY (region_id) REFERENCES regions (id);

COMMENT ON CONSTRAINT events_region_fk ON events IS 'parent foreign key, updated';

COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';

COMMENT ON CONSTRAINT events_region_fk ON events_leaf IS 'child foreign key';

COMMENT ON CONSTRAINT events_old_pkey ON events_old IS NULL;
