CREATE TABLE IF NOT EXISTS orders (
    id integer,
    label text,
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);

CREATE SEQUENCE IF NOT EXISTS orders_id_seq AS integer OWNED BY orders.id;
