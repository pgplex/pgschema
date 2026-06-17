CREATE TABLE public.orders (
    order_id bigserial PRIMARY KEY,
    customer_id integer NOT NULL,
    total numeric(10,2) NOT NULL
);

COMMENT ON SEQUENCE public.orders_order_id_seq IS 'Primary key sequence for the orders table';
