--
-- pgschema database dump
--

-- Dumped from database version PostgreSQL 17.6
-- Dumped by pgschema version 1.14.1


--
-- Name: products; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS products (
    sku text,
    name text,
    CONSTRAINT products_pkey PRIMARY KEY (sku),
    CONSTRAINT products_name_key UNIQUE (name)
);
ALTER TABLE products
ADD CONSTRAINT products_name_unique UNIQUE (name);
ALTER TABLE products
ADD CONSTRAINT products_sku_key UNIQUE (sku);


COMMENT ON CONSTRAINT products_sku_key ON products IS 'target of the order line foreign key';

--
-- Name: order_lines; Type: TABLE; Schema: -; Owner: -
--

CREATE TABLE IF NOT EXISTS order_lines (
    id integer,
    sku text NOT NULL,
    CONSTRAINT order_lines_pkey PRIMARY KEY (id),
    CONSTRAINT order_lines_sku_fkey FOREIGN KEY (sku) REFERENCES products (sku) ON DELETE CASCADE
);

