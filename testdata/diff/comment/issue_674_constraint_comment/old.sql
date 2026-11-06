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

COMMENT ON CONSTRAINT customers_email_key ON customers IS 'comment to drop';
COMMENT ON CONSTRAINT orders_pkey ON orders IS 'order identifier';
COMMENT ON CONSTRAINT orders_total_check ON orders IS 'old wording';

CREATE TABLE regions (
    id integer,
    CONSTRAINT regions_pkey PRIMARY KEY (id)
);

CREATE TABLE events (
    id integer PRIMARY KEY,
    region_id integer,
    CONSTRAINT events_region_fk FOREIGN KEY (region_id) REFERENCES regions (id)
) PARTITION BY RANGE (id);
CREATE TABLE events_leaf PARTITION OF events FOR VALUES FROM (0) TO (10);
CREATE TABLE events_old PARTITION OF events FOR VALUES FROM (10) TO (20);

COMMENT ON CONSTRAINT events_pkey ON events IS 'parent key';
COMMENT ON CONSTRAINT events_leaf_pkey ON events_leaf IS 'child key';
COMMENT ON CONSTRAINT events_old_pkey ON events_old IS 'comment to drop on a retained key';
-- Renaming regions_pkey drops and re-adds events_region_fk, recreating the
-- partition's copy without its comment; the plan must restore both.
COMMENT ON CONSTRAINT events_region_fk ON events IS 'parent foreign key';
COMMENT ON CONSTRAINT events_region_fk ON events_leaf IS 'child foreign key';
