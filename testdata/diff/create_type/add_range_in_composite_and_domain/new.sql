-- A composite attribute of the range and a domain over its multirange: the
-- range is created first even though both dependents sort before it by name.
CREATE TYPE period AS RANGE (subtype = date);

CREATE TYPE booking AS (room integer, slot period);

CREATE DOMAIN availability AS period_multirange;
