-- The root adds a column and only the grandchild has a change of its own,
-- an index on that column; it must follow the root's ADD COLUMN although
-- the table in between has no changes.
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
