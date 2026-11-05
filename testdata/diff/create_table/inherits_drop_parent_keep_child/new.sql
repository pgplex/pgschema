-- The parent goes away and the child stands alone, keeping the columns it
-- inherited but not the inherited CHECK constraint.
CREATE TABLE capitals (
    name text NOT NULL,
    population real,
    elevation integer,
    state char(2) NOT NULL
);
