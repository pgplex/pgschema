CREATE TABLE products (
    sku text PRIMARY KEY,
    name text
);
ALTER TABLE products ADD CONSTRAINT products_sku_key UNIQUE (sku);

CREATE TABLE order_lines (
    id integer PRIMARY KEY,
    sku text NOT NULL REFERENCES products (sku)
);
