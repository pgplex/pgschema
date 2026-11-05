-- z_parent adds region and a foreign key to a_child's new unique key, while
-- a_child indexes the inherited region: the index must follow the parent's
-- ADD COLUMN even though the foreign key points the other way.
CREATE TABLE z_parent (
    id integer NOT NULL,
    favorite_uid integer,
    region text
);

CREATE TABLE a_child (
    uid integer NOT NULL,
    CONSTRAINT a_child_uid_key UNIQUE (uid)
) INHERITS (z_parent);

CREATE INDEX a_child_region_idx ON a_child (region);

ALTER TABLE z_parent
    ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid);
