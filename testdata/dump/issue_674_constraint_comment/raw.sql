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
    CONSTRAINT orders_customer_id_fkey FOREIGN KEY (customer_id) REFERENCES customers (id)
);

COMMENT ON CONSTRAINT customers_email_key ON customers IS 'one account per email';
COMMENT ON CONSTRAINT orders_pkey ON orders IS 'order identifier';
COMMENT ON CONSTRAINT orders_total_check ON orders IS 'totals are never negative';
COMMENT ON CONSTRAINT orders_customer_id_fkey ON orders IS 'owning customer';

CREATE TABLE events (id integer PRIMARY KEY) PARTITION BY RANGE (id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);

COMMENT ON CONSTRAINT events_pkey ON events IS 'parent key';
COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';
