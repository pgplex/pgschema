CREATE TABLE p_root (
    id integer NOT NULL,
    legacy text
);

CREATE TABLE m_middle (
    note text
) INHERITS (p_root);

CREATE TABLE g_leaf (
    detail text
) INHERITS (m_middle);
