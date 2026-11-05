-- Changes to existing inheritance hierarchies: parent-driven column and
-- constraint changes, parents dropping columns and CHECK constraints, and
-- tables joining and leaving parents.


-- Parent-driven column and constraint changes on existing hierarchies.

-- events gains two columns and a CHECK: the children get them from the
-- parent. alerts (existing) and pages (new) set their own DEFAULT / NOT
-- NULL / comment on priority after the parent's ADD COLUMN, and alerts
-- indexes the new country column.
CREATE TABLE events (
    id integer NOT NULL,
    population real,
    priority integer DEFAULT 1,
    country text DEFAULT 'US' NOT NULL,
    CONSTRAINT events_pop_check CHECK (population >= 0)
);

CREATE TABLE alerts (
    channel text
) INHERITS (events);

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;
ALTER TABLE alerts ALTER COLUMN priority SET NOT NULL;
COMMENT ON COLUMN alerts.priority IS 'Paging priority for this alert';

CREATE INDEX alerts_country_idx ON alerts (country);

CREATE TABLE pages (
    pager text
) INHERITS (events);

ALTER TABLE ONLY pages ALTER COLUMN priority SET DEFAULT 3;

-- regions retypes population and drops elevation and its CHECK; all three
-- recurse to provinces without a statement of its own.
CREATE TABLE regions (
    name text NOT NULL,
    population bigint,
    CONSTRAINT regions_pkey PRIMARY KEY (name)
);

CREATE TABLE provinces (
    code char(2) NOT NULL
) INHERITS (regions);

-- tickets' new default recurses down to outages, which inherits 9 from
-- incidents; both get their desired default back.
CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 2
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 9;

CREATE TABLE outages (
    region text
) INHERITS (incidents);

-- batches' own default changes with ONLY; runs is created before that and
-- inherits the old one, so it gets its own.
CREATE TABLE jobs (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE batches (
    name text
) INHERITS (jobs);

ALTER TABLE ONLY batches ALTER COLUMN priority SET DEFAULT 3;

CREATE TABLE runs (
    started_at timestamptz
) INHERITS (batches);

-- readings' new expression recurses to scaled_readings, whose own
-- expression is then put back.
CREATE TABLE readings (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 4) STORED
);

CREATE TABLE scaled_readings (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (readings);

-- Only calibrated_samples' expression changes; its own child keeps raw * 3,
-- so the change must not recurse.
CREATE TABLE samples (
    raw integer NOT NULL,
    scaled integer GENERATED ALWAYS AS (raw * 2) STORED
);

CREATE TABLE calibrated_samples (
    scaled integer GENERATED ALWAYS AS (raw * 4) STORED
) INHERITS (samples);

CREATE TABLE recalibrated_samples (
    scaled integer GENERATED ALWAYS AS (raw * 3) STORED
) INHERITS (calibrated_samples);

-- contacts stops requiring phone while users still does, and users makes
-- the inherited email NOT NULL: the online rewrite on PostgreSQL 14-17
-- names its temporary CHECK <column>_not_null, which the inherited CHECK
-- already occupies, so it must pick another name.
CREATE TABLE contacts (
    id integer NOT NULL,
    email text,
    phone text,
    CONSTRAINT email_not_null CHECK (email <> '')
);

CREATE TABLE users (
    nickname text
) INHERITS (contacts);

ALTER TABLE users ALTER COLUMN phone SET NOT NULL;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;

-- z_root adds a column and only the grandchild has a change of its own, an
-- index on it, which must follow the root's ADD COLUMN although middle has
-- no changes.
CREATE TABLE z_root (
    id integer NOT NULL,
    region text
);

CREATE TABLE middle (
    level_two text
) INHERITS (z_root);

CREATE TABLE a_leaf (
    level_three text
) INHERITS (middle);

CREATE INDEX a_leaf_region_idx ON a_leaf (region);

-- z_hub adds region and a foreign key to a_spoke's new unique key, while
-- a_spoke indexes the inherited region: the index must follow the parent's
-- ADD COLUMN even though the foreign key points the other way.
CREATE TABLE z_hub (
    id integer NOT NULL,
    favorite_uid integer,
    region text
);

CREATE TABLE a_spoke (
    uid integer NOT NULL,
    CONSTRAINT a_spoke_uid_key UNIQUE (uid)
) INHERITS (z_hub);

CREATE INDEX a_spoke_region_idx ON a_spoke (region);

ALTER TABLE z_hub
    ADD CONSTRAINT z_hub_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_spoke (uid);

-- villages changes what it sets locally on inherited columns: a new default
-- and NOT NULL on population, and the elevation default goes.
CREATE TABLE towns (
    name text NOT NULL,
    population real,
    elevation integer,
    CONSTRAINT towns_pkey PRIMARY KEY (name)
);

CREATE TABLE villages (
    hamlet boolean
) INHERITS (towns);

ALTER TABLE ONLY villages ALTER COLUMN population SET DEFAULT 1;
ALTER TABLE ONLY villages ALTER COLUMN population SET NOT NULL;


-- Parents dropping columns and CHECK constraints that descendants keep or
-- must drop themselves.

-- wheels moves to cars alone: vehicles drops it with ONLY so cars keeps the
-- column and its data; boats drops its copy, and so does trucks, which is
-- created before the drop and inherits the column.
CREATE TABLE vehicles (
    id integer NOT NULL
);

CREATE TABLE cars (
    wheels integer,
    doors integer
) INHERITS (vehicles);

CREATE TABLE boats (
    draft numeric
) INHERITS (vehicles);

CREATE TABLE trucks (
    payload numeric
) INHERITS (vehicles);

-- products drops legacy_code and its CHECK; the copies books declared
-- survive the parent's drops and are dropped by books.
CREATE TABLE products (
    id integer NOT NULL,
    price numeric
);

CREATE TABLE books (
    isbn text
) INHERITS (products);

-- shared goes from all three tables: the child's merged copy can only be
-- dropped once both parents have dropped theirs.
CREATE TABLE shared_a (
    label_a text
);

CREATE TABLE shared_b (
    label_b text
);

CREATE TABLE shared_child (
    extra text
) INHERITS (shared_a, shared_b);

-- common goes everywhere; m_middle and g_leaf declared it as well, so they
-- drop their copies themselves, parents first.
CREATE TABLE a_base (
    label_a text
);

CREATE TABLE z_base (
    label_z text
);

CREATE TABLE m_middle (
    note text
) INHERITS (z_base);

CREATE TABLE g_leaf (
    detail text
) INHERITS (a_base, m_middle);

-- legacy moves to the grandchild: root_docs and mid_docs drop it with ONLY
-- so leaf_docs keeps the column.
CREATE TABLE root_docs (
    id integer NOT NULL
);

CREATE TABLE mid_docs (
    note text
) INHERITS (root_docs);

CREATE TABLE leaf_docs (
    legacy text,
    detail text
) INHERITS (mid_docs);

-- check_a drops shared_check; check_child still inherits it from check_b,
-- so its copy stays.
CREATE TABLE check_a (
    id integer NOT NULL
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

-- attach_child joins attach_parent: the parent's CHECK it lacks is added
-- before the INHERIT.
CREATE TABLE attach_parent (
    name text NOT NULL,
    population real,
    CONSTRAINT attach_parent_pkey PRIMARY KEY (name),
    CONSTRAINT attach_parent_pop_check CHECK (population >= 0)
);

CREATE TABLE attach_child (
    state char(2) NOT NULL
) INHERITS (attach_parent);

-- archived_gauge_readings joins gauge_readings: its same-named CHECK with another
-- expression is replaced before the INHERIT.
CREATE TABLE gauge_readings (
    value integer,
    CONSTRAINT gauge_readings_value_check CHECK (value > 10)
);

CREATE TABLE archived_gauge_readings (
    archived_at timestamptz
) INHERITS (gauge_readings);

-- moving_child moves from base_a to base_b: label_a becomes local on NO
-- INHERIT and is dropped, label_b and base_b's CHECK are needed before
-- INHERIT.
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
) INHERITS (base_b);

-- old_parent goes away and orphan stands alone, keeping the columns it
-- inherited but not the inherited CHECK.
CREATE TABLE orphan (
    name text NOT NULL,
    population real,
    state char(2) NOT NULL
);

-- multi_a gains a column while multi_child leaves multi_b: the new column
-- reaches multi_child through multi_a, label_b becomes local and is
-- dropped, and multi_shared_check stays inherited from multi_a.
CREATE TABLE multi_a (
    id integer NOT NULL,
    label_a text,
    added text,
    CONSTRAINT multi_shared_check CHECK (id > 0)
);

CREATE TABLE multi_b (
    id integer NOT NULL,
    label_b text,
    CONSTRAINT multi_shared_check CHECK (id > 0)
);

CREATE TABLE multi_child (
    extra integer
) INHERITS (multi_a);

-- join_child already has join_shared_check through join_a, so joining
-- join_b needs nothing but the INHERIT.
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
) INHERITS (join_a, join_b);

-- queue_alerts joins queue_events in the same migration that adds priority
-- to queue_events; it brings its own priority (with its own default).
CREATE TABLE queue_events (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE queue_alerts (
    channel text
) INHERITS (queue_events);

ALTER TABLE ONLY queue_alerts ALTER COLUMN priority SET DEFAULT 2;

-- fleet_cars keeps wheels, so fleet drops it with ONLY; fleet_boats leaves
-- fleet and drops wheels through its own changes.
CREATE TABLE fleet (
    id integer NOT NULL
);

CREATE TABLE fleet_cars (
    wheels integer,
    doors integer
) INHERITS (fleet);

CREATE TABLE fleet_boats (
    id integer NOT NULL,
    draft numeric
);
