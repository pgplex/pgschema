CREATE TABLE a_base (
    shared integer,
    label_a text
);

CREATE TABLE z_base (
    shared integer,
    label_z text
);

CREATE TABLE m_middle (
    shared integer,
    note text
) INHERITS (z_base);

CREATE TABLE g_leaf (
    shared integer,
    detail text
) INHERITS (a_base, m_middle);
