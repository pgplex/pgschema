CREATE TABLE IF NOT EXISTS orders (
    order_id BIGSERIAL,
    customer_id integer NOT NULL,
    total numeric(10,2) NOT NULL,
    CONSTRAINT orders_pkey PRIMARY KEY (order_id)
);
COMMENT ON SEQUENCE orders_order_id_seq IS 'Primary key sequence for the orders table';
