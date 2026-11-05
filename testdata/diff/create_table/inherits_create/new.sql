-- Creating inheritance hierarchies from scratch.

-- Single parent, a column the child declares as well (elevation), a local
-- default on a purely inherited column (population), a child-only CHECK and
-- index, and a grandchild.
CREATE TABLE cities (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT cities_pkey PRIMARY KEY (name),
    CONSTRAINT cities_pop_check CHECK (population >= 0)
);

CREATE TABLE capitals (
    state char(2) NOT NULL,
    elevation integer DEFAULT 0 NOT NULL,
    CONSTRAINT capitals_state_check CHECK (state <> '')
) INHERITS (cities);

ALTER TABLE ONLY capitals ALTER COLUMN population SET DEFAULT 1;

CREATE INDEX capitals_state_idx ON capitals (state);

CREATE TABLE state_capitals (
    founded date
) INHERITS (capitals);

-- Multiple parents
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

-- Parents disagreeing on a default: the child must settle it (x), and keeps
-- the declaration even after dropping its own default (defaults_cleared).
-- A default only one parent supplies (y) is inherited and can be dropped.
CREATE TABLE defaults_a (
    x integer DEFAULT 1,
    y integer
);

CREATE TABLE defaults_b (
    x integer DEFAULT 2,
    y integer DEFAULT 5
);

CREATE TABLE defaults_child (
    x integer DEFAULT 3,
    extra text
) INHERITS (defaults_a, defaults_b);

ALTER TABLE ONLY defaults_child ALTER COLUMN y DROP DEFAULT;

CREATE TABLE defaults_cleared (
    x integer DEFAULT 3
) INHERITS (defaults_a, defaults_b);

ALTER TABLE ONLY defaults_cleared ALTER COLUMN x DROP DEFAULT;

-- A child giving a generated column its own expression, and parents that
-- generate a column differently, which the child must settle.
CREATE TABLE measurements (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED
);

CREATE TABLE calibrated (
    scaled integer GENERATED ALWAYS AS (raw * 10 + 1) STORED,
    device text
) INHERITS (measurements);

CREATE TABLE sensor_a (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED
);

CREATE TABLE sensor_b (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 20) STORED
);

CREATE TABLE combined (
    scaled integer GENERATED ALWAYS AS (raw * 10) STORED,
    label text
) INHERITS (sensor_a, sensor_b);

-- A CHECK the child declares as well as inherits keeps its declaration
CREATE TABLE products (
    id integer NOT NULL,
    price numeric,
    CONSTRAINT products_price_check CHECK (price >= 0)
);

CREATE TABLE books (
    isbn text,
    CONSTRAINT products_price_check CHECK (price >= 0)
) INHERITS (products);

-- The parent's CHECK needs a new function, which puts the parent in a later
-- creation batch; the child must follow it there.
CREATE FUNCTION is_valid_code(code text) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$ SELECT code ~ '^[A-Z]{3}$' $$;

CREATE TABLE coded_parent (
    code text NOT NULL,
    CONSTRAINT coded_parent_code_check CHECK (is_valid_code(code))
);

CREATE TABLE coded_child (
    detail text
) INHERITS (coded_parent);

-- a_child inherits from b_parent and references it, and z_parent references
-- a_child: the inheritance edges must survive the foreign key cycle break.
CREATE TABLE b_parent (
    id integer NOT NULL,
    CONSTRAINT b_parent_pkey PRIMARY KEY (id)
);

CREATE TABLE z_parent (
    code text NOT NULL,
    favorite_uid integer
);

CREATE TABLE a_child (
    uid integer NOT NULL,
    parent_id integer,
    CONSTRAINT a_child_uid_key UNIQUE (uid),
    CONSTRAINT a_child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES b_parent (id)
) INHERITS (b_parent, z_parent);

ALTER TABLE z_parent
    ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid);
