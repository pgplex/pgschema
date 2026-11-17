-- The form that pgschema dump writes for the schema of old.sql
CREATE TABLE IF NOT EXISTS products (
    sku text,
    name text,
    CONSTRAINT products_pkey PRIMARY KEY (sku)
);
ALTER TABLE products
ADD CONSTRAINT products_sku_key UNIQUE (sku);

CREATE TABLE IF NOT EXISTS order_lines (
    id integer,
    sku text NOT NULL,
    CONSTRAINT order_lines_pkey PRIMARY KEY (id),
    CONSTRAINT order_lines_sku_fkey FOREIGN KEY (sku) REFERENCES products (sku)
);
