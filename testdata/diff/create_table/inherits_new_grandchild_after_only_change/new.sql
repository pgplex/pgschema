-- outages is created before incidents' default changes and inherits the old
-- one; the ONLY change on incidents does not reach it, so it gets its own.
CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 3;

CREATE TABLE outages (
    region text
) INHERITS (incidents);
