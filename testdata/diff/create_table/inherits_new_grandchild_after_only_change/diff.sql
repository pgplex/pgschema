CREATE TABLE IF NOT EXISTS outages (
    region text
) INHERITS (incidents);

ALTER TABLE ONLY incidents ALTER COLUMN priority SET DEFAULT 3;

ALTER TABLE ONLY outages ALTER COLUMN priority SET DEFAULT 3;
