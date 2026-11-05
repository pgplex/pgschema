CREATE TABLE IF NOT EXISTS z_parent (
    id integer NOT NULL,
    primary_child_uid integer
);

CREATE TABLE IF NOT EXISTS a_child (
    uid integer NOT NULL,
    CONSTRAINT a_child_uid_key UNIQUE (uid)
) INHERITS (z_parent);

ALTER TABLE z_parent
ADD CONSTRAINT z_parent_primary_child_uid_fkey FOREIGN KEY (primary_child_uid) REFERENCES a_child (uid);
