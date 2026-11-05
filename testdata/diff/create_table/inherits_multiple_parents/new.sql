CREATE TABLE audited (
    created_at timestamptz DEFAULT now() NOT NULL,
    created_by text
);

CREATE TABLE located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK (region <> '')
);

CREATE TABLE orders (
    id integer NOT NULL,
    amount numeric(10,2),
    CONSTRAINT orders_pkey PRIMARY KEY (id)
) INHERITS (audited, located);
