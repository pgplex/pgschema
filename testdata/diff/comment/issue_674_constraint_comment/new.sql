CREATE TABLE customers (
    id integer PRIMARY KEY,
    email text,
    CONSTRAINT customers_email_key UNIQUE (email)
);

CREATE TABLE orders (
    id integer PRIMARY KEY,
    customer_id integer NOT NULL,
    total numeric NOT NULL,
    CONSTRAINT orders_total_check CHECK (total >= 0),
    CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id),
    CONSTRAINT orders_customer_total_key UNIQUE (customer_id, total)
);

COMMENT ON CONSTRAINT orders_pkey ON orders IS 'order identifier';
COMMENT ON CONSTRAINT orders_total_check ON orders IS 'totals are never negative';
COMMENT ON CONSTRAINT orders_customer_id_fkey ON orders IS 'owning customer';
COMMENT ON CONSTRAINT orders_customer_total_key ON orders IS 'added with its constraint';

CREATE TABLE invoices (
    id integer PRIMARY KEY,
    CONSTRAINT invoices_id_check CHECK (id > 0)
);

COMMENT ON CONSTRAINT invoices_pkey ON invoices IS 'invoice identifier';
COMMENT ON CONSTRAINT invoices_id_check ON invoices IS 'positive identifiers only';

CREATE TABLE regions (
    id integer,
    CONSTRAINT regions_id_key PRIMARY KEY (id)
);

CREATE TABLE events (
    id integer PRIMARY KEY,
    region_id integer,
    CONSTRAINT events_region_fk FOREIGN KEY (region_id) REFERENCES regions (id)
) PARTITION BY RANGE (id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

COMMENT ON CONSTRAINT events_pkey ON events IS 'parent key';
-- Renaming regions_pkey drops and re-adds events_region_fk, recreating the
-- partition's copy without its comment; the plan must restore both.
COMMENT ON CONSTRAINT events_region_fk ON events IS 'parent foreign key';
COMMENT ON CONSTRAINT events_region_fk ON events_leaf IS 'child foreign key';
COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';
