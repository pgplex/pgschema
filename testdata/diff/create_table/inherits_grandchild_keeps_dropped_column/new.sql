-- legacy moves to the grandchild: the root and the middle table drop it with
-- ONLY so the grandchild keeps the column and its data.
CREATE TABLE p_root (
    id integer NOT NULL
);

CREATE TABLE m_middle (
    note text
) INHERITS (p_root);

CREATE TABLE g_leaf (
    legacy text,
    detail text
) INHERITS (m_middle);
