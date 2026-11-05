-- a_child both inherits from b_parent and references it, and z_parent
-- references a_child: the inheritance edges must survive the cycle break.
CREATE TABLE b_parent (
    id integer NOT NULL,
    CONSTRAINT b_parent_pkey PRIMARY KEY (id)
);

CREATE TABLE z_parent (
    code text NOT NULL,
    favorite_uid integer
);

CREATE TABLE a_child (
    uid integer NOT NULL,
    parent_id integer,
    CONSTRAINT a_child_uid_key UNIQUE (uid),
    CONSTRAINT a_child_parent_id_fkey FOREIGN KEY (parent_id) REFERENCES b_parent (id)
) INHERITS (b_parent, z_parent);

ALTER TABLE z_parent
    ADD CONSTRAINT z_parent_favorite_uid_fkey FOREIGN KEY (favorite_uid) REFERENCES a_child (uid);
