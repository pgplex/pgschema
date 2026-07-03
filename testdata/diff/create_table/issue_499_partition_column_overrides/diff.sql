CREATE TABLE IF NOT EXISTS orders (
    id bigint NOT NULL,
    region text NOT NULL,
    priority integer DEFAULT 0,
    notes text
) PARTITION BY LIST (region);

CREATE TABLE IF NOT EXISTS orders_eu PARTITION OF orders FOR VALUES IN ('eu');

CREATE TABLE IF NOT EXISTS orders_us PARTITION OF orders (
    priority DEFAULT 10,
    notes NOT NULL
) FOR VALUES IN ('us');
