-- Function whose return type changes (DROP + CREATE)
CREATE FUNCTION default_qty(x integer) RETURNS integer LANGUAGE sql IMMUTABLE AS $$ SELECT x $$;

-- Partitioned table: the default, CHECK constraint and index of the parent
-- call the function. Partition p1 keeps a default of its own, p2 and p3 copy
-- the parent's; the partitions' copies of the CHECK and index follow the parent.
CREATE TABLE measurements (
    id integer NOT NULL,
    qty integer DEFAULT default_qty(1),
    CONSTRAINT measurements_qty_check CHECK (qty <= default_qty(100))
) PARTITION BY RANGE (id);
CREATE TABLE measurements_p1 PARTITION OF measurements (qty DEFAULT 7) FOR VALUES FROM (0) TO (10);
CREATE TABLE measurements_p2 PARTITION OF measurements FOR VALUES FROM (10) TO (20);
CREATE TABLE measurements_p3 PARTITION OF measurements FOR VALUES FROM (20) TO (30);
CREATE INDEX measurements_qty_idx ON measurements ((qty + default_qty(0)));
