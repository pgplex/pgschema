CREATE TABLE tickets (
    id integer NOT NULL,
    priority integer DEFAULT 1
);

CREATE TABLE incidents (
    severity text
) INHERITS (tickets);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 2;
