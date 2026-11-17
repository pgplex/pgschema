-- CREATE TABLE drops a key whose index equals an earlier one, so the
-- duplicate keys are added with ALTER TABLE.
CREATE TABLE products (
    sku text PRIMARY KEY,
    name text,
    CONSTRAINT products_name_key UNIQUE (name)
);
ALTER TABLE products ADD CONSTRAINT products_sku_key UNIQUE (sku);
ALTER TABLE products ADD CONSTRAINT products_name_unique UNIQUE (name);

CREATE TABLE order_lines (
    id integer PRIMARY KEY,
    sku text NOT NULL REFERENCES products (sku)
);
