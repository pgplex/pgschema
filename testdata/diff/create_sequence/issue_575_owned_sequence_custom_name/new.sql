CREATE SEQUENCE custom_seq INCREMENT BY 5;

CREATE TABLE orders (
    id bigint PRIMARY KEY DEFAULT nextval('custom_seq'),
    label text
);

ALTER SEQUENCE custom_seq OWNED BY orders.id;
