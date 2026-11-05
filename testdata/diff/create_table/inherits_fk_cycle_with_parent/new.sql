-- a_child inherits from z_parent, and z_parent references a key that only
-- a_child has: the foreign key can wait, the INHERITS clause cannot, so
-- z_parent is created first despite the alphabetical order.
CREATE TABLE z_parent (
    id integer NOT NULL,
    primary_child_uid integer
);

CREATE TABLE a_child (
    uid integer NOT NULL,
    CONSTRAINT a_child_uid_key UNIQUE (uid)
) INHERITS (z_parent);

ALTER TABLE z_parent
    ADD CONSTRAINT z_parent_primary_child_uid_fkey FOREIGN KEY (primary_child_uid) REFERENCES a_child (uid);
