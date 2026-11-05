ALTER TABLE fleet_boats NO INHERIT fleet;

ALTER TABLE moving_child NO INHERIT base_a;

ALTER TABLE multi_child NO INHERIT multi_b;

ALTER TABLE orphan NO INHERIT old_parent;

DROP TABLE IF EXISTS old_parent CASCADE;

CREATE TABLE IF NOT EXISTS pages (
    pager text
) INHERITS (events);

CREATE TABLE IF NOT EXISTS runs (
    started_at timestamptz
) INHERITS (batches);

CREATE TABLE IF NOT EXISTS trucks (
    payload numeric
) INHERITS (vehicles);

ALTER TABLE a_base DROP COLUMN common;

ALTER TABLE archived_gauge_readings DROP CONSTRAINT gauge_readings_value_check;

ALTER TABLE ONLY batches ALTER COLUMN priority SET DEFAULT 3;

ALTER TABLE ONLY runs ALTER COLUMN priority SET DEFAULT 3;

ALTER TABLE ONLY calibrated_samples ALTER COLUMN scaled SET EXPRESSION AS ((raw * 4));

ALTER TABLE check_a DROP CONSTRAINT shared_check;

ALTER TABLE contacts ALTER COLUMN phone DROP NOT NULL;

ALTER TABLE events ADD COLUMN priority integer DEFAULT 1;

ALTER TABLE events ADD COLUMN country text DEFAULT 'US' NOT NULL;

ALTER TABLE alerts ADD CONSTRAINT alerts_priority_not_null NOT NULL priority NOT VALID;

ALTER TABLE alerts VALIDATE CONSTRAINT alerts_priority_not_null;

ALTER TABLE ONLY alerts ALTER COLUMN priority SET DEFAULT 2;

ALTER TABLE ONLY pages ALTER COLUMN priority SET DEFAULT 3;

ALTER TABLE events
ADD CONSTRAINT events_pop_check CHECK (population >= 0::double precision) NOT VALID;

ALTER TABLE events VALIDATE CONSTRAINT events_pop_check;

COMMENT ON COLUMN alerts.priority IS 'Paging priority for this alert';

CREATE INDEX CONCURRENTLY IF NOT EXISTS alerts_country_idx ON alerts (country);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'alerts_country_idx'
  AND n.nspname = 'public';

ALTER TABLE ONLY fleet DROP COLUMN wheels;

ALTER TABLE fleet_boats DROP COLUMN wheels;

ALTER TABLE moving_child DROP COLUMN label_a;

ALTER TABLE moving_child ADD COLUMN label_b text;

ALTER TABLE multi_a ADD COLUMN added text;

ALTER TABLE multi_child DROP COLUMN label_b;

ALTER TABLE orphan DROP CONSTRAINT old_parent_pop_check;

ALTER TABLE products DROP CONSTRAINT products_price_check;

ALTER TABLE products DROP COLUMN legacy_code;

ALTER TABLE queue_events ADD COLUMN priority integer DEFAULT 1;

ALTER TABLE readings ALTER COLUMN scaled SET EXPRESSION AS ((raw * 4));

ALTER TABLE ONLY scaled_readings ALTER COLUMN scaled SET EXPRESSION AS ((raw * 3));

ALTER TABLE regions DROP CONSTRAINT regions_pop_check;

ALTER TABLE regions DROP COLUMN elevation;

ALTER TABLE regions ALTER COLUMN population TYPE bigint USING population::bigint;

ALTER TABLE ONLY root_docs DROP COLUMN legacy;

ALTER TABLE shared_a DROP COLUMN shared;

ALTER TABLE shared_b DROP COLUMN shared;

ALTER TABLE tickets ALTER COLUMN priority SET DEFAULT 2;

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 9;

ALTER TABLE ONLY outages ALTER COLUMN priority SET DEFAULT 9;

ALTER TABLE users ADD CONSTRAINT users_email_not_null NOT NULL email NOT VALID;

ALTER TABLE users VALIDATE CONSTRAINT users_email_not_null;

ALTER TABLE ONLY vehicles DROP COLUMN wheels;

ALTER TABLE villages ADD CONSTRAINT villages_population_not_null NOT NULL population NOT VALID;

ALTER TABLE villages VALIDATE CONSTRAINT villages_population_not_null;

ALTER TABLE ONLY villages ALTER COLUMN population SET DEFAULT 1;

ALTER TABLE ONLY villages ALTER COLUMN elevation DROP DEFAULT;

ALTER TABLE z_base DROP COLUMN common;

ALTER TABLE z_root ADD COLUMN region text;

CREATE INDEX CONCURRENTLY IF NOT EXISTS a_leaf_region_idx ON a_leaf (region);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'a_leaf_region_idx'
  AND n.nspname = 'public';

ALTER TABLE z_hub ADD COLUMN region text;

ALTER TABLE a_spoke
ADD CONSTRAINT a_spoke_uid_key UNIQUE (uid);

CREATE INDEX CONCURRENTLY IF NOT EXISTS a_spoke_region_idx ON a_spoke (region);

-- pgschema:wait
SELECT 
    COALESCE(i.indisvalid, false) as done,
    CASE 
        WHEN p.blocks_total > 0 THEN p.blocks_done * 100 / p.blocks_total
        ELSE 0
    END as progress
FROM pg_class c
JOIN pg_namespace n ON c.relnamespace = n.oid
LEFT JOIN pg_index i ON c.oid = i.indexrelid
LEFT JOIN pg_stat_progress_create_index p ON c.oid = p.index_relid
WHERE c.relname = 'a_spoke_region_idx'
  AND n.nspname = 'public';

ALTER TABLE boats DROP COLUMN wheels;

ALTER TABLE books DROP COLUMN legacy_code;

ALTER TABLE m_middle DROP COLUMN common;

ALTER TABLE ONLY mid_docs DROP COLUMN legacy;

ALTER TABLE shared_child DROP COLUMN shared;

ALTER TABLE trucks DROP COLUMN wheels;

ALTER TABLE g_leaf DROP COLUMN common;

ALTER TABLE books DROP CONSTRAINT products_price_check;

ALTER TABLE archived_gauge_readings
ADD CONSTRAINT gauge_readings_value_check CHECK (value > 10) NOT VALID;

ALTER TABLE archived_gauge_readings VALIDATE CONSTRAINT gauge_readings_value_check;

ALTER TABLE archived_gauge_readings INHERIT gauge_readings;

ALTER TABLE attach_child
ADD CONSTRAINT attach_parent_pop_check CHECK (population >= 0::double precision) NOT VALID;

ALTER TABLE attach_child VALIDATE CONSTRAINT attach_parent_pop_check;

ALTER TABLE attach_child INHERIT attach_parent;

ALTER TABLE join_child INHERIT join_b;

ALTER TABLE moving_child
ADD CONSTRAINT base_b_id_check CHECK (id > 0) NOT VALID;

ALTER TABLE moving_child VALIDATE CONSTRAINT base_b_id_check;

ALTER TABLE moving_child INHERIT base_b;

ALTER TABLE queue_alerts INHERIT queue_events;

ALTER TABLE z_hub
ADD CONSTRAINT z_hub_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_spoke (uid) NOT VALID;

ALTER TABLE z_hub VALIDATE CONSTRAINT z_hub_favorite_uid_fkey;
