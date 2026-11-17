CREATE TABLE products (
    sku text PRIMARY KEY,
    name text,
    CONSTRAINT products_name_key UNIQUE (name)
);

-- CREATE TABLE drops a key whose index equals an earlier one, so these
-- duplicate keys are added with ALTER TABLE.
ALTER TABLE products ADD CONSTRAINT products_sku_key UNIQUE (sku);
ALTER TABLE products ADD CONSTRAINT products_name_unique UNIQUE (name);

COMMENT ON CONSTRAINT products_sku_key ON products IS 'target of the order line foreign key';

CREATE TABLE order_lines (
    id integer PRIMARY KEY,
    sku text NOT NULL,
    CONSTRAINT order_lines_sku_fkey FOREIGN KEY (sku) REFERENCES products (sku) ON DELETE CASCADE
);
