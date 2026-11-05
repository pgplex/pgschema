CREATE TABLE IF NOT EXISTS audited (
    created_at timestamptz DEFAULT now() NOT NULL,
    created_by text
);

CREATE TABLE IF NOT EXISTS located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK (region <> ''::text)
);

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    amount numeric(10,2),
    CONSTRAINT orders_pkey PRIMARY KEY (id)
) INHERITS (audited, located);
