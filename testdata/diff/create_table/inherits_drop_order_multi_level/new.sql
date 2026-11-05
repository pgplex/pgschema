-- shared goes everywhere. m_middle and g_leaf declared it as well, so they
-- drop their copies themselves, parents first: g_leaf's copy is still
-- inherited from m_middle until m_middle has dropped its own.
CREATE TABLE a_base (
    label_a text
);

CREATE TABLE z_base (
    label_z text
);

CREATE TABLE m_middle (
    note text
) INHERITS (z_base);

CREATE TABLE g_leaf (
    detail text
) INHERITS (a_base, m_middle);
