ALTER TABLE z_parent ADD COLUMN region text;

ALTER TABLE a_child
ADD CONSTRAINT a_child_uid_key UNIQUE (uid);

CREATE INDEX IF NOT EXISTS a_child_region_idx ON a_child (region);

ALTER TABLE z_parent
ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid);
