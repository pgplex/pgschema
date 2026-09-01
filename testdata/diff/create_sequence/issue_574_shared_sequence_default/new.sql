CREATE SEQUENCE shared_seq;

CREATE TABLE orders (
    id bigint NOT NULL DEFAULT nextval('shared_seq'),
    PRIMARY KEY (id)
);

CREATE TABLE invoices (
    id bigint NOT NULL DEFAULT nextval('shared_seq'),
    PRIMARY KEY (id)
);
