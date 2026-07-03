CREATE TABLE IF NOT EXISTS orders_us PARTITION OF orders (
    priority DEFAULT 10,
    notes NOT NULL
) FOR VALUES IN ('us');
