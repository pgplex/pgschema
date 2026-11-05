ALTER TABLE cities ADD COLUMN country text DEFAULT 'US' NOT NULL;

ALTER TABLE cities
ADD CONSTRAINT cities_pop_check CHECK (population >= 0::double precision) NOT VALID;

ALTER TABLE cities VALIDATE CONSTRAINT cities_pop_check;

CREATE INDEX CONCURRENTLY IF NOT EXISTS capitals_country_idx ON capitals (country);

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
WHERE c.relname = 'capitals_country_idx'
  AND n.nspname = 'public';
