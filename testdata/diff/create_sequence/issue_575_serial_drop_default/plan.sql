CREATE SEQUENCE IF NOT EXISTS orders_id_seq AS integer;

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    label text,
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);

ALTER SEQUENCE orders_id_seq OWNED BY orders.id;
