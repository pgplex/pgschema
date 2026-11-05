CREATE TABLE z_root (
    id integer NOT NULL
);

CREATE TABLE middle (
    level_two text
) INHERITS (z_root);

CREATE TABLE a_leaf (
    level_three text
) INHERITS (middle);
