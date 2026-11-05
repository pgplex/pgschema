CREATE TABLE z_parent (
    id integer NOT NULL,
    favorite_uid integer
);

CREATE TABLE a_child (
    uid integer NOT NULL
) INHERITS (z_parent);
