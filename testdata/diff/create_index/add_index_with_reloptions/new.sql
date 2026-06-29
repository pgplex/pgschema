CREATE TABLE public.orders (
    id serial PRIMARY KEY,
    user_id integer NOT NULL,
    status varchar(50),
    created_at timestamp DEFAULT CURRENT_TIMESTAMP
);

-- btree index with fillfactor storage parameter
CREATE INDEX idx_orders_user_id ON public.orders (user_id) WITH (fillfactor=90);

-- partial index with fillfactor and deduplicate_items (WITH must come before WHERE)
CREATE INDEX idx_orders_status_active ON public.orders (user_id, created_at) WITH (fillfactor=100, deduplicate_items=true) WHERE status IS NOT NULL;
