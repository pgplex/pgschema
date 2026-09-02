CREATE SEQUENCE orders_id_seq;

CREATE TABLE orders (
    id bigint PRIMARY KEY DEFAULT nextval('orders_id_seq') + 100,
    label text
);

ALTER SEQUENCE orders_id_seq OWNED BY orders.id;
