CREATE SEQUENCE orders_id_seq;

CREATE TABLE orders (
    id bigint NOT NULL DEFAULT nextval('orders_id_seq'),
    PRIMARY KEY (id)
);
