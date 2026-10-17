ALTER TABLE measurements DROP CONSTRAINT IF EXISTS measurements_qty_check;

DROP INDEX IF EXISTS measurements_qty_idx;

ALTER TABLE ONLY measurements ALTER COLUMN qty DROP DEFAULT;

ALTER TABLE measurements_p2 ALTER COLUMN qty DROP DEFAULT;

ALTER TABLE measurements_p3 ALTER COLUMN qty DROP DEFAULT;

DROP FUNCTION IF EXISTS default_qty(integer);

CREATE OR REPLACE FUNCTION default_qty(
    x integer
)
RETURNS bigint
LANGUAGE sql
IMMUTABLE
AS $$ SELECT x
$$;

ALTER TABLE ONLY measurements ALTER COLUMN qty SET DEFAULT default_qty(1);

ALTER TABLE measurements
ADD CONSTRAINT measurements_qty_check CHECK (qty <= default_qty(100)) NOT VALID;

CREATE INDEX IF NOT EXISTS measurements_qty_idx ON measurements ((qty + default_qty(0)));

ALTER TABLE measurements_p2 ALTER COLUMN qty SET DEFAULT default_qty(1);

ALTER TABLE measurements_p3 ALTER COLUMN qty SET DEFAULT default_qty(1);

ALTER TABLE measurements VALIDATE CONSTRAINT measurements_qty_check;
