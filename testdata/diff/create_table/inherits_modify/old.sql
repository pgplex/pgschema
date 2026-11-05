-- Changes to existing inheritance hierarchies: parent-driven column and
-- constraint changes, parents dropping columns and CHECK constraints, and
-- tables joining and leaving parents.


-- Parent-driven column and constraint changes on existing hierarchies.

CREATE TABLE events (
    id integer NOT NULL,
    population real
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

CREATE TABLE regions (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT regions_pkey PRIMARY KEY (name),
    CONSTRAINT regions_pop_check CHECK (population >= 0)
);

CREATE TABLE provinces (
    code char(2) NOT NULL
) INHERITS (regions);

CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 9;

CREATE TABLE outages (
    region text
) INHERITS (incidents);

CREATE TABLE jobs (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE batches (
    name text
) INHERITS (jobs);

ALTER TABLE ONLY batches ALTER COLUMN priority SET DEFAULT 2;

CREATE TABLE readings (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 2) STORED
);

CREATE TABLE scaled_readings (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (readings);

CREATE TABLE samples (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 2) STORED
);

CREATE TABLE calibrated_samples (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (samples);

CREATE TABLE recalibrated_samples (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (calibrated_samples);

CREATE TABLE contacts (
    id integer NOT NULL,
    email text,
    phone text NOT NULL,
    CONSTRAINT email_not_null CHECK (email <> '')
);

CREATE TABLE users (
    nickname text
) INHERITS (contacts);

ALTER TABLE users ALTER COLUMN phone SET NOT NULL;

CREATE TABLE z_root (
    id integer NOT NULL
);

CREATE TABLE middle (
    level_two text
) INHERITS (z_root);

CREATE TABLE a_leaf (
    level_three text
) INHERITS (middle);

CREATE TABLE z_hub (
    id integer NOT NULL,
    favorite_uid integer
);

CREATE TABLE a_spoke (
    uid integer NOT NULL
) INHERITS (z_hub);

CREATE TABLE towns (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT towns_pkey PRIMARY KEY (name)
);

CREATE TABLE villages (
    hamlet boolean
) INHERITS (towns);

ALTER TABLE ONLY villages ALTER COLUMN elevation SET DEFAULT 0;


-- Parents dropping columns and CHECK constraints that descendants keep or
-- must drop themselves.

CREATE TABLE vehicles (
    id integer NOT NULL,
    wheels integer
);

CREATE TABLE cars (
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);

CREATE TABLE products (
    id integer NOT NULL,
    price numeric,
    legacy_code text,
    CONSTRAINT products_price_check CHECK (price >= 0)
);

-- books declares legacy_code and the CHECK as well, so the parent's drops
-- leave them on books as local objects
CREATE TABLE books (
    legacy_code text,
    isbn text,
    CONSTRAINT products_price_check CHECK (price >= 0)
) INHERITS (products);

CREATE TABLE shared_a (
    shared integer,
    label_a text
);

CREATE TABLE shared_b (
    shared integer,
    label_b text
);

CREATE TABLE shared_child (
    shared integer,
    extra text
) INHERITS (shared_a, shared_b);

CREATE TABLE a_base (
    common integer,
    label_a text
);

CREATE TABLE z_base (
    common integer,
    label_z text
);

CREATE TABLE m_middle (
    common integer,
    note text
) INHERITS (z_base);

CREATE TABLE g_leaf (
    common integer,
    detail text
) INHERITS (a_base, m_middle);

CREATE TABLE root_docs (
    id integer NOT NULL,
    legacy text
);

CREATE TABLE mid_docs (
    note text
) INHERITS (root_docs);

CREATE TABLE leaf_docs (
    detail text
) INHERITS (mid_docs);

CREATE TABLE check_a (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE check_b (
    id integer NOT NULL,
    CONSTRAINT shared_check CHECK (id > 0)
);

CREATE TABLE check_child (
    extra text,
    CONSTRAINT shared_check CHECK (id > 0)
) INHERITS (check_a, check_b);


-- Tables joining and leaving inheritance parents.

CREATE TABLE attach_parent (
    name text NOT NULL,
    population real,
    CONSTRAINT attach_parent_pkey PRIMARY KEY (name),
    CONSTRAINT attach_parent_pop_check CHECK (population >= 0)
);

-- A standalone table with the parent's columns but not its CHECK
CREATE TABLE attach_child (
    name text NOT NULL,
    population real,
    state char(2) NOT NULL
);

CREATE TABLE gauge_readings (
    value integer,
    CONSTRAINT gauge_readings_value_check CHECK (value > 10)
);

-- A standalone table whose same-named CHECK differs from the parent's
CREATE TABLE archived_gauge_readings (
    value integer,
    archived_at timestamptz,
    CONSTRAINT gauge_readings_value_check CHECK (value > 0)
);

CREATE TABLE base_a (
    id integer NOT NULL,
    label_a text
);

CREATE TABLE base_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT base_b_id_check CHECK (id > 0)
);

CREATE TABLE moving_child (
    extra integer
) INHERITS (base_a);

CREATE TABLE old_parent (
    name text NOT NULL,
    population real,
    CONSTRAINT old_parent_pop_check CHECK (population >= 0)
);

CREATE TABLE orphan (
    state char(2) NOT NULL
) INHERITS (old_parent);

CREATE TABLE multi_a (
    id integer NOT NULL,
    label_a text,
    CONSTRAINT multi_shared_check CHECK (id > 0)
);

CREATE TABLE multi_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT multi_shared_check CHECK (id > 0)
);

CREATE TABLE multi_child (
    extra integer
) INHERITS (multi_a, multi_b);

CREATE TABLE join_a (
    id integer NOT NULL,
    CONSTRAINT join_shared_check CHECK (id > 0)
);

CREATE TABLE join_b (
    id integer NOT NULL,
    CONSTRAINT join_shared_check CHECK (id > 0)
);

CREATE TABLE join_child (
    extra text
) INHERITS (join_a);

CREATE TABLE queue_events (
    id integer NOT NULL
);

-- A standalone table that already has the column the parent is about to add
CREATE TABLE queue_alerts (
    id integer NOT NULL,
    priority integer DEFAULT 2,
    channel text
);

CREATE TABLE fleet (
    id integer NOT NULL,
    wheels integer
);

CREATE TABLE fleet_cars (
    doors integer
) INHERITS (fleet);

CREATE TABLE fleet_boats (
    draft numeric
) INHERITS (fleet);
