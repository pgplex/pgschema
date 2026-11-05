CREATE TABLE IF NOT EXISTS audited (
    created_at timestamptz DEFAULT now() NOT NULL,
    created_by text
);

CREATE TABLE IF NOT EXISTS b_parent (
    id integer,
    CONSTRAINT b_parent_pkey PRIMARY KEY (id)
);

CREATE TABLE IF NOT EXISTS cities (
    name text,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0::double precision)
);

CREATE TABLE IF NOT EXISTS capitals (
    elevation integer DEFAULT 0 NOT NULL,
    state character(2) NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> ''::bpchar)
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

CREATE INDEX IF NOT EXISTS capitals_state_idx ON capitals (state);

CREATE TABLE IF NOT EXISTS defaults_a (
    x integer DEFAULT 1,
    y integer
);

CREATE TABLE IF NOT EXISTS defaults_b (
    x integer DEFAULT 2,
    y integer DEFAULT 5
);

CREATE TABLE IF NOT EXISTS defaults_child (
    x integer DEFAULT 3,
    extra text
) INHERITS (defaults_a, defaults_b);

ALTER TABLE ONLY defaults_child ALTER COLUMN y DROP DEFAULT;

CREATE TABLE IF NOT EXISTS defaults_cleared (
    x integer DEFAULT NULL
) INHERITS (defaults_a, defaults_b);

ALTER TABLE ONLY defaults_cleared ALTER COLUMN x DROP DEFAULT;

CREATE TABLE IF NOT EXISTS located (
    region text NOT NULL,
    CONSTRAINT located_region_check CHECK (region <> ''::text)
);

CREATE TABLE IF NOT EXISTS measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED
);

CREATE TABLE IF NOT EXISTS calibrated (
    scaled integer GENERATED ALWAYS AS (((raw * 10) + 1)) STORED,
    device text
) INHERITS (measurements);

CREATE TABLE IF NOT EXISTS orders (
    id integer,
    amount numeric(10,2),
    CONSTRAINT orders_pkey PRIMARY KEY (id)
) INHERITS (audited, located);

CREATE TABLE IF NOT EXISTS products (
    id integer NOT NULL,
    price numeric,
    CONSTRAINT products_price_check CHECK (price >= 0::numeric)
);

CREATE TABLE IF NOT EXISTS books (
    isbn text,
    CONSTRAINT products_price_check CHECK (price >= 0::numeric)
) INHERITS (products);

CREATE TABLE IF NOT EXISTS sensor_a (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED
);

CREATE TABLE IF NOT EXISTS sensor_b (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS ((raw * 20)) STORED
);

CREATE TABLE IF NOT EXISTS combined (
    scaled integer GENERATED ALWAYS AS ((raw * 10)) STORED,
    label text
) INHERITS (sensor_a, sensor_b);

CREATE TABLE IF NOT EXISTS state_capitals (
    founded date
) INHERITS (capitals);

CREATE TABLE IF NOT EXISTS z_parent (
    code text NOT NULL,
    favorite_uid integer
);

CREATE TABLE IF NOT EXISTS a_child (
    uid integer NOT NULL,
    parent_id integer,
    CONSTRAINT a_child_uid_key UNIQUE (uid),
    CONSTRAINT a_child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES b_parent (id)
) INHERITS (b_parent, z_parent);

CREATE OR REPLACE FUNCTION is_valid_code(
    code text
)
RETURNS boolean
LANGUAGE sql
IMMUTABLE
AS $$ SELECT code ~ '^[A-Z]{3}$'
$$;

CREATE TABLE IF NOT EXISTS coded_parent (
    code text NOT NULL,
    CONSTRAINT coded_parent_code_check CHECK (is_valid_code(code))
);

CREATE TABLE IF NOT EXISTS coded_child (
    detail text
) INHERITS (coded_parent);

ALTER TABLE z_parent
ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid);
