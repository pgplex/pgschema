-- The parent's new default recurses down to outages, which inherits 9 from
-- incidents; both descendants get their desired default back.
CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 2
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 9;

CREATE TABLE outages (
    region text
) INHERITS (incidents);
