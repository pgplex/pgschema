CREATE SEQUENCE IF NOT EXISTS shared_seq;

CREATE TABLE IF NOT EXISTS invoices (
    id bigint DEFAULT nextval('shared_seq'::regclass),
    CONSTRAINT invoices_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS orders (
    id bigint DEFAULT nextval('shared_seq'::regclass),
    CONSTRAINT orders_pkey PRIMARY KEY (id)
);
