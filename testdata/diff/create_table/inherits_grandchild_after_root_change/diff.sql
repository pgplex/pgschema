ALTER TABLE z_root ADD COLUMN region text;

CREATE INDEX IF NOT EXISTS a_leaf_region_idx ON a_leaf (region);
