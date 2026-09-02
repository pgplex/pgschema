CREATE SEQUENCE IF NOT EXISTS custom_seq INCREMENT BY 5;

CREATE TABLE IF NOT EXISTS orders (
    id bigint DEFAULT nextval('custom_seq'::regclass),
    label text,
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);

ALTER SEQUENCE custom_seq OWNED BY orders.id;
