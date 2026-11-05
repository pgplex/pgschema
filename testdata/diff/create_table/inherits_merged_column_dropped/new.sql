CREATE TABLE products (
    id integer NOT NULL
);

CREATE TABLE books (
    isbn text
) INHERITS (products);
