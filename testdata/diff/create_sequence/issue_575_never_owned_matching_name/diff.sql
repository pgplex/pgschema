CREATE SEQUENCE IF NOT EXISTS orders_id_seq;

CREATE TABLE IF NOT EXISTS orders (
    id bigint DEFAULT nextval('orders_id_seq'::regclass),
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);
