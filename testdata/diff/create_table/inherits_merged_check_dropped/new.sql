CREATE TABLE products (
    id integer NOT NULL,
    price numeric
);

CREATE TABLE books (
    isbn text
) INHERITS (products);
