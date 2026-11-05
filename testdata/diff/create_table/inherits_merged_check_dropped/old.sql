CREATE TABLE products (
    id integer NOT NULL,
    price numeric,
    CONSTRAINT products_price_check CHECK (price >= 0)
);

-- The child declares the parent's CHECK as well, so the parent's drop leaves
-- the child's copy behind as a local constraint
CREATE TABLE books (
    isbn text,
    CONSTRAINT products_price_check CHECK (price >= 0)
) INHERITS (products);
