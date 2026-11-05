CREATE TABLE products (
    id integer NOT NULL,
    legacy_code text
);

-- legacy_code is declared again in the child, so the parent's DROP COLUMN
-- leaves it on the child as a local column
CREATE TABLE books (
    legacy_code text,
    isbn text
) INHERITS (products);
