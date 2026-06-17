CREATE INDEX IF NOT EXISTS idx_orders_status_active ON orders (user_id, created_at) WITH (fillfactor=100, deduplicate_items=true) WHERE (status IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id) WITH (fillfactor=90);
