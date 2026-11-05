-- The parent's new default recurses to incidents; the child's own default
-- is re-applied afterwards.
CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 2
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 9;
